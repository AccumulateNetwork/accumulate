// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataTx_MarshalUnmarshal(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	tx := NewDataTx(pub, []byte("hello world"), 1)
	tx.Sign(priv)

	data := tx.Marshal()
	tx2, err := UnmarshalTransaction(data)
	require.NoError(t, err)

	dataTx, ok := tx2.(*DataTx)
	require.True(t, ok)

	assert.Equal(t, tx.Hash(), dataTx.Hash())
	assert.True(t, dataTx.Verify())
}

func TestSetBlockTimeTx_MarshalUnmarshal(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	tx := NewSetBlockTimeTx(pub, 5*time.Second, 1)
	tx.Sign(priv)

	data := tx.Marshal()
	tx2, err := UnmarshalTransaction(data)
	require.NoError(t, err)

	blockTx, ok := tx2.(*SetBlockTimeTx)
	require.True(t, ok)

	assert.Equal(t, tx.Hash(), blockTx.Hash())
	assert.Equal(t, 5*time.Second, blockTx.BlockInterval)
	assert.True(t, blockTx.Verify())
}

func TestSetTxRateTx_MarshalUnmarshal(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	tx := NewSetTxRateTx(pub, 500, 1)
	tx.Sign(priv)

	data := tx.Marshal()
	tx2, err := UnmarshalTransaction(data)
	require.NoError(t, err)

	rateTx, ok := tx2.(*SetTxRateTx)
	require.True(t, ok)

	assert.Equal(t, tx.Hash(), rateTx.Hash())
	assert.Equal(t, uint32(500), rateTx.TxPerSecond)
	assert.True(t, rateTx.Verify())
}

func TestBlock_MarshalUnmarshal(t *testing.T) {
	block := &Block{
		Height:    42,
		PrevHash:  [32]byte{1, 2, 3},
		Timestamp: time.Now().Truncate(time.Nanosecond),
		TxnCount:  100,
		TxnsHash:  [32]byte{4, 5, 6},
		StateHash: [32]byte{7, 8, 9},
	}

	data := block.Marshal()
	block2, err := UnmarshalBlock(data)
	require.NoError(t, err)

	assert.Equal(t, block.Height, block2.Height)
	assert.Equal(t, block.PrevHash, block2.PrevHash)
	assert.Equal(t, block.Timestamp, block2.Timestamp)
	assert.Equal(t, block.TxnCount, block2.TxnCount)
	assert.Equal(t, block.TxnsHash, block2.TxnsHash)
	assert.Equal(t, block.StateHash, block2.StateHash)
}

func TestExecutor_ProcessTransaction(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	executor, err := NewExecutor(ExecutorConfig{
		Validators:    []ed25519.PublicKey{pub},
		BlockInterval: 1 * time.Second,
		TxRate:        100,
	})
	require.NoError(t, err)

	// Process a data transaction
	tx := NewDataTx(pub, []byte("test data"), 1)
	tx.Sign(priv)

	err = executor.ProcessTransaction(tx.Marshal())
	require.NoError(t, err)

	assert.Equal(t, uint64(1), executor.GetProcessedCount())
}

func TestExecutor_SetBlockTime(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	executor, err := NewExecutor(ExecutorConfig{
		Validators:    []ed25519.PublicKey{pub},
		BlockInterval: 1 * time.Second,
		TxRate:        100,
	})
	require.NoError(t, err)

	assert.Equal(t, 1*time.Second, executor.GetBlockInterval())

	// Submit SetBlockTime transaction
	tx := NewSetBlockTimeTx(pub, 5*time.Second, 1)
	tx.Sign(priv)

	err = executor.ProcessTransaction(tx.Marshal())
	require.NoError(t, err)

	assert.Equal(t, 5*time.Second, executor.GetBlockInterval())
	assert.Equal(t, uint64(1), executor.GetProcessedCount())
}

func TestExecutor_SetBlockTime_NonValidator(t *testing.T) {
	validatorPub, _, _ := ed25519.GenerateKey(rand.Reader)
	userPub, userPriv, _ := ed25519.GenerateKey(rand.Reader)

	executor, err := NewExecutor(ExecutorConfig{
		Validators:    []ed25519.PublicKey{validatorPub},
		BlockInterval: 1 * time.Second,
		TxRate:        100,
	})
	require.NoError(t, err)

	// Non-validator tries to change block time
	tx := NewSetBlockTimeTx(userPub, 10*time.Second, 1)
	tx.Sign(userPriv)

	err = executor.ProcessTransaction(tx.Marshal())
	require.NoError(t, err) // No error, but change is rejected

	// Block time should be unchanged
	assert.Equal(t, 1*time.Second, executor.GetBlockInterval())
	assert.Equal(t, uint64(0), executor.GetProcessedCount())
}

func TestExecutor_BlockProduction(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	executor, err := NewExecutor(ExecutorConfig{
		Validators:    []ed25519.PublicKey{pub},
		BlockInterval: 100 * time.Millisecond,
		TxRate:        100,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var blocksProduced atomic.Int32
	executor.SetOnBlockProduced(func(b *Block) {
		blocksProduced.Add(1)
	})

	// Submit some transactions
	for i := 0; i < 10; i++ {
		tx := NewDataTx(pub, []byte("test"), uint64(i+1))
		tx.Sign(priv)
		executor.ProcessTransaction(tx.Marshal())
	}

	executor.Start(ctx)
	time.Sleep(500 * time.Millisecond)
	executor.Stop()

	// Should have produced at least a few blocks
	assert.Greater(t, blocksProduced.Load(), int32(0))
	assert.Greater(t, executor.GetBlockCount(), uint64(1)) // genesis + at least one
}

func TestExecutor_ReplayProtection(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	executor, err := NewExecutor(ExecutorConfig{
		Validators:    []ed25519.PublicKey{pub},
		BlockInterval: 1 * time.Second,
		TxRate:        100,
	})
	require.NoError(t, err)

	// Submit transaction with nonce 1
	tx1 := NewDataTx(pub, []byte("first"), 1)
	tx1.Sign(priv)
	err = executor.ProcessTransaction(tx1.Marshal())
	require.NoError(t, err)
	assert.Equal(t, uint64(1), executor.GetProcessedCount())

	// Try to replay the same transaction
	err = executor.ProcessTransaction(tx1.Marshal())
	require.NoError(t, err)
	assert.Equal(t, uint64(1), executor.GetProcessedCount()) // Still 1

	// Submit with higher nonce should work
	tx2 := NewDataTx(pub, []byte("second"), 2)
	tx2.Sign(priv)
	err = executor.ProcessTransaction(tx2.Marshal())
	require.NoError(t, err)
	assert.Equal(t, uint64(2), executor.GetProcessedCount())
}

func TestGenesisBlock(t *testing.T) {
	genesis := GenesisBlock()

	assert.Equal(t, uint64(0), genesis.Height)
	assert.Equal(t, [32]byte{}, genesis.PrevHash)
	assert.Equal(t, uint32(0), genesis.TxnCount)
}

func TestComputeTxnsHash(t *testing.T) {
	// Empty list
	hash1 := ComputeTxnsHash(nil)
	hash2 := ComputeTxnsHash([][32]byte{})
	assert.Equal(t, hash1, hash2)

	// Single transaction
	txHash := [32]byte{1, 2, 3}
	hash3 := ComputeTxnsHash([][32]byte{txHash})
	assert.NotEqual(t, hash1, hash3)

	// Order matters
	txHash2 := [32]byte{4, 5, 6}
	hash4 := ComputeTxnsHash([][32]byte{txHash, txHash2})
	hash5 := ComputeTxnsHash([][32]byte{txHash2, txHash})
	assert.NotEqual(t, hash4, hash5)
}
