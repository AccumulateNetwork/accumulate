// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package adapter_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// mockBlockProducer implements BlockProducer for testing.
type mockBlockProducer struct {
	producedBlocks []adapter.BlockParams
	nextHash       [32]byte
}

func (m *mockBlockProducer) ProduceBlock(ctx context.Context, params adapter.BlockParams) ([32]byte, error) {
	m.producedBlocks = append(m.producedBlocks, params)
	return m.nextHash, nil
}

// mockTransactionValidator implements TransactionValidator for testing.
type mockTransactionValidator struct {
	validatedTxs [][]byte
	validErr     error
}

func (m *mockTransactionValidator) ValidateTransaction(tx []byte) error {
	m.validatedTxs = append(m.validatedTxs, tx)
	return m.validErr
}

// mockStateProvider implements StateProvider for testing.
type mockStateProvider struct {
	index uint64
	hash  [32]byte
}

func (m *mockStateProvider) LastBlock() (uint64, [32]byte, error) {
	return m.index, m.hash, nil
}

func (m *mockStateProvider) StateHash() [32]byte {
	return m.hash
}

// mockValidatorSetProvider implements ValidatorSetProvider for testing.
type mockValidatorSetProvider struct {
	validators []adapter.ValidatorInfo
	callback   func([]adapter.ValidatorInfo)
}

func (m *mockValidatorSetProvider) Validators() []adapter.ValidatorInfo {
	return m.validators
}

func (m *mockValidatorSetProvider) OnValidatorSetChange(callback func([]adapter.ValidatorInfo)) {
	m.callback = callback
}

// Verify interface compliance at compile time
var (
	_ adapter.BlockProducer         = (*mockBlockProducer)(nil)
	_ adapter.TransactionValidator  = (*mockTransactionValidator)(nil)
	_ adapter.StateProvider         = (*mockStateProvider)(nil)
	_ adapter.ValidatorSetProvider  = (*mockValidatorSetProvider)(nil)
)

func TestBlockParams(t *testing.T) {
	// Test that BlockParams can be created and fields are accessible
	params := adapter.BlockParams{
		Index:       100,
		Time:        time.Now(),
		IsLeader:    true,
		LeaderRound: types.Round(50),
		Certificate: nil, // Can be nil for testing
		Batches:     make(map[types.BatchDigest]*types.Batch),
	}

	assert.Equal(t, uint64(100), params.Index)
	assert.True(t, params.IsLeader)
	assert.Equal(t, types.Round(50), params.LeaderRound)
}

func TestValidatorInfo(t *testing.T) {
	info := adapter.ValidatorInfo{
		PublicKey: [32]byte{1, 2, 3},
		Stake:     1000,
		Active:    true,
	}

	assert.Equal(t, uint64(1000), info.Stake)
	assert.True(t, info.Active)
}

func TestMockBlockProducer(t *testing.T) {
	mock := &mockBlockProducer{
		nextHash: [32]byte{1, 2, 3, 4},
	}

	params := adapter.BlockParams{
		Index:    1,
		IsLeader: true,
	}

	hash, err := mock.ProduceBlock(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, mock.nextHash, hash)
	assert.Len(t, mock.producedBlocks, 1)
	assert.Equal(t, params.Index, mock.producedBlocks[0].Index)
}

func TestMockTransactionValidator(t *testing.T) {
	mock := &mockTransactionValidator{}

	tx := []byte("test transaction")
	err := mock.ValidateTransaction(tx)
	require.NoError(t, err)
	assert.Len(t, mock.validatedTxs, 1)
	assert.Equal(t, tx, mock.validatedTxs[0])
}

func TestMockStateProvider(t *testing.T) {
	expectedHash := [32]byte{0xab, 0xcd}
	mock := &mockStateProvider{
		index: 42,
		hash:  expectedHash,
	}

	index, hash, err := mock.LastBlock()
	require.NoError(t, err)
	assert.Equal(t, uint64(42), index)
	assert.Equal(t, expectedHash, hash)
	assert.Equal(t, expectedHash, mock.StateHash())
}

func TestMockValidatorSetProvider(t *testing.T) {
	validators := []adapter.ValidatorInfo{
		{PublicKey: [32]byte{1}, Stake: 100, Active: true},
		{PublicKey: [32]byte{2}, Stake: 200, Active: true},
	}

	mock := &mockValidatorSetProvider{
		validators: validators,
	}

	assert.Equal(t, validators, mock.Validators())

	mock.OnValidatorSetChange(func(v []adapter.ValidatorInfo) {
		// Callback registered
	})
	assert.NotNil(t, mock.callback)
}
