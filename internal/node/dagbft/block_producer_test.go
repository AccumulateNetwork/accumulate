// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// mockExecutor implements execute.Executor for testing.
type mockExecutor struct {
	lastBlockParams *execute.BlockParams
	lastBlockHash   [32]byte
	validateError   error
	beginError      error
}

func (m *mockExecutor) EnableTimers() {}

func (m *mockExecutor) StoreBlockTimers(ds *logging.DataSet) {}

func (m *mockExecutor) LastBlock() (*execute.BlockParams, [32]byte, error) {
	return m.lastBlockParams, m.lastBlockHash, nil
}

func (m *mockExecutor) Init(validators []*execute.ValidatorUpdate) ([]*execute.ValidatorUpdate, error) {
	return nil, nil
}

func (m *mockExecutor) Validate(envelope *messaging.Envelope, recheck bool) ([]*protocol.TransactionStatus, error) {
	if m.validateError != nil {
		return nil, m.validateError
	}
	return []*protocol.TransactionStatus{}, nil
}

func (m *mockExecutor) Begin(params execute.BlockParams) (execute.Block, error) {
	if m.beginError != nil {
		return nil, m.beginError
	}
	return &mockBlock{params: params}, nil
}

// mockBlock implements execute.Block for testing.
type mockBlock struct {
	params     execute.BlockParams
	txCount    int
	closeError error
}

func (b *mockBlock) Params() execute.BlockParams {
	return b.params
}

func (b *mockBlock) Process(envelope *messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	b.txCount++
	return []*protocol.TransactionStatus{}, nil
}

func (b *mockBlock) Close() (execute.BlockState, error) {
	if b.closeError != nil {
		return nil, b.closeError
	}
	return &mockBlockState{params: b.params, txCount: b.txCount}, nil
}

// mockBlockState implements execute.BlockState for testing.
type mockBlockState struct {
	params    execute.BlockParams
	txCount   int
	committed bool
	discarded bool
}

func (s *mockBlockState) Params() execute.BlockParams {
	return s.params
}

func (s *mockBlockState) IsEmpty() bool {
	return s.txCount == 0
}

func (s *mockBlockState) DidCompleteMajorBlock() (uint64, time.Time, bool) {
	return 0, time.Time{}, false
}

func (s *mockBlockState) DidUpdateValidators() ([]*execute.ValidatorUpdate, bool) {
	return nil, false
}

func (s *mockBlockState) ChangeSet() record.Record {
	return nil
}

func (s *mockBlockState) Hash() ([32]byte, error) {
	return [32]byte{1, 2, 3, 4}, nil
}

func (s *mockBlockState) Commit() error {
	s.committed = true
	return nil
}

func (s *mockBlockState) Discard() {
	s.discarded = true
}

func TestBlockProducer_ProduceBlock_Empty(t *testing.T) {
	executor := &mockExecutor{}
	producer := NewBlockProducer(BlockProducerConfig{
		Executor:  executor,
		Partition: "test",
	})

	// Create empty block params (no batches)
	params := BlockParams{
		Index:       1,
		Time:        time.Now(),
		IsLeader:    true,
		LeaderRound: 2,
		Certificate: nil,
		Batches:     map[types.BatchDigest]*types.Batch{},
	}

	hash, err := producer.ProduceBlock(context.Background(), params)
	require.NoError(t, err)
	// Empty blocks return the previous hash (which is zero for first block)
	require.Equal(t, [32]byte{}, hash)
}

func TestBlockProducer_LastBlock(t *testing.T) {
	executor := &mockExecutor{
		lastBlockParams: &execute.BlockParams{Index: 10},
		lastBlockHash:   [32]byte{5, 6, 7, 8},
	}
	producer := NewBlockProducer(BlockProducerConfig{
		Executor:  executor,
		Partition: "test",
	})

	index, hash, err := producer.LastBlock()
	require.NoError(t, err)
	require.Equal(t, uint64(0), index) // BlockProducer tracks its own state
	require.Equal(t, [32]byte{}, hash)
}

func TestNewDAGConsensusNode(t *testing.T) {
	// Create minimal test components
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	committee := types.NewCommittee([]types.ValidatorInfo{
		{PublicKey: pub, Stake: 1},
	}, 1)

	d := dag.NewDAG(100)
	executor := &mockExecutor{}

	config := NodeConfig{
		Partition:  "test",
		PrivateKey: priv,
	}

	node, err := NewDAGConsensusNode(config, executor, nil, committee, d, nil)
	require.NoError(t, err)
	require.NotNil(t, node)
	require.NotNil(t, node.blockProducer)
	require.NotNil(t, node.bullshark)
}

func TestDAGConsensusNode_IsReady(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	committee := types.NewCommittee([]types.ValidatorInfo{
		{PublicKey: pub, Stake: 1},
	}, 1)

	d := dag.NewDAG(100)
	executor := &mockExecutor{}

	config := NodeConfig{
		Partition:  "test",
		PrivateKey: priv,
	}

	node, err := NewDAGConsensusNode(config, executor, nil, committee, d, nil)
	require.NoError(t, err)

	// Not ready before start
	require.False(t, node.IsReady())

	// Start the node
	ctx, cancel := context.WithCancel(context.Background())
	err = node.Start(ctx)
	require.NoError(t, err)

	// Ready after start
	require.True(t, node.IsReady())

	// Stop the node
	cancel()
	err = node.Stop()
	require.NoError(t, err)
}
