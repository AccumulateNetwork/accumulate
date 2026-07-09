// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testutil

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"time"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	tmtypes "github.com/cometbft/cometbft/proto/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	acmerrors "gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// MockExecutor implements execute.Executor for testing.
type MockExecutor struct {
	mu         sync.RWMutex
	blocks     []*MockBlock
	lastIndex  uint64
	lastHash   [32]byte
	validators []*execute.ValidatorUpdate
	timersOn   bool

	// Hooks for customizing behavior
	OnBegin   func(params execute.BlockParams) error
	OnProcess func(env *messaging.Envelope) error
	OnClose   func() error
	OnCommit  func() error
}

// MockBlock represents a block being built.
type MockBlock struct {
	mu       sync.Mutex
	params   execute.BlockParams
	txs      [][]byte
	state    *MockBlockState
	executor *MockExecutor
}

// MockBlockState represents the state of a completed block.
type MockBlockState struct {
	mu        sync.Mutex
	block     *MockBlock
	hash      [32]byte
	committed bool
	params    execute.BlockParams
	empty     bool
}

// NewMockExecutor creates a new mock executor.
func NewMockExecutor() *MockExecutor {
	return &MockExecutor{
		blocks: make([]*MockBlock, 0),
	}
}

// EnableTimers enables timing.
func (e *MockExecutor) EnableTimers() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.timersOn = true
}

// StoreBlockTimers stores block timing data.
func (e *MockExecutor) StoreBlockTimers(ds *logging.DataSet) {
	// No-op for mock
}

// LastBlock returns the height and hash of the last block.
func (e *MockExecutor) LastBlock() (*execute.BlockParams, [32]byte, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	params := &execute.BlockParams{
		Context:  context.Background(),
		Index:    e.lastIndex,
		Time:     time.Now(),
		IsLeader: true,
	}
	return params, e.lastHash, nil
}

// Init initializes the executor with validators.
func (e *MockExecutor) Init(validators []*execute.ValidatorUpdate) ([]*execute.ValidatorUpdate, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.validators = validators
	return nil, nil
}

// Validate validates an envelope.
func (e *MockExecutor) Validate(envelope *messaging.Envelope, recheck bool) ([]*protocol.TransactionStatus, error) {
	// Basic validation - always pass for mock
	return []*protocol.TransactionStatus{
		{Code: acmerrors.Delivered},
	}, nil
}

// Begin begins a new block.
func (e *MockExecutor) Begin(params execute.BlockParams) (execute.Block, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.OnBegin != nil {
		if err := e.OnBegin(params); err != nil {
			return nil, err
		}
	}

	block := &MockBlock{
		params:   params,
		txs:      make([][]byte, 0),
		executor: e,
	}
	block.state = &MockBlockState{
		block:  block,
		params: params,
	}
	e.blocks = append(e.blocks, block)

	return block, nil
}

// GetBlocks returns all blocks created by this executor.
func (e *MockExecutor) GetBlocks() []*MockBlock {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]*MockBlock, len(e.blocks))
	copy(result, e.blocks)
	return result
}

// BlockCount returns the number of blocks.
func (e *MockExecutor) BlockCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.blocks)
}

// SetValidators sets the validator updates.
func (e *MockExecutor) SetValidators(validators []*execute.ValidatorUpdate) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.validators = validators
}

// Params returns the block parameters.
func (b *MockBlock) Params() execute.BlockParams {
	return b.params
}

// Process processes an envelope and adds transactions to the block.
func (b *MockBlock) Process(env *messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.executor.OnProcess != nil {
		if err := b.executor.OnProcess(env); err != nil {
			return nil, err
		}
	}

	// Marshal the envelope and add to transactions
	data, err := env.MarshalBinary()
	if err != nil {
		return nil, err
	}
	b.txs = append(b.txs, data)

	return []*protocol.TransactionStatus{
		{Code: acmerrors.Delivered},
	}, nil
}

// Close closes the block and computes its hash.
func (b *MockBlock) Close() (execute.BlockState, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.executor.OnClose != nil {
		if err := b.executor.OnClose(); err != nil {
			return nil, err
		}
	}

	// Compute deterministic hash from transactions
	h := sha256.New()
	for _, tx := range b.txs {
		h.Write(tx)
	}
	copy(b.state.hash[:], h.Sum(nil))

	b.state.empty = len(b.txs) == 0

	return b.state, nil
}

// GetTransactions returns all transactions in the block.
func (b *MockBlock) GetTransactions() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([][]byte, len(b.txs))
	copy(result, b.txs)
	return result
}

// TransactionCount returns the number of transactions.
func (b *MockBlock) TransactionCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.txs)
}

// Params returns the block parameters.
func (s *MockBlockState) Params() execute.BlockParams {
	return s.params
}

// IsEmpty returns whether the block is empty.
func (s *MockBlockState) IsEmpty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.empty
}

// DidCompleteMajorBlock returns whether a major block was completed.
func (s *MockBlockState) DidCompleteMajorBlock() (uint64, time.Time, bool) {
	return 0, time.Time{}, false
}

// DidUpdateValidators returns validator updates if any.
func (s *MockBlockState) DidUpdateValidators() ([]*execute.ValidatorUpdate, bool) {
	return nil, false
}

// ChangeSet returns the database batch (nil for mock).
func (s *MockBlockState) ChangeSet() record.Record {
	return nil
}

// Hash returns the block hash.
func (s *MockBlockState) Hash() ([32]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hash, nil
}

// Commit commits the block.
func (s *MockBlockState) Commit() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.block.executor.OnCommit != nil {
		if err := s.block.executor.OnCommit(); err != nil {
			return err
		}
	}

	s.committed = true

	// Update executor state
	s.block.executor.mu.Lock()
	s.block.executor.lastIndex = s.params.Index
	s.block.executor.lastHash = s.hash
	s.block.executor.mu.Unlock()

	return nil
}

// Discard discards the block.
func (s *MockBlockState) Discard() {
	s.mu.Lock()
	defer s.mu.Unlock()
	// No-op for mock
}

// IsCommitted returns whether the block was committed.
func (s *MockBlockState) IsCommitted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.committed
}

// Compile-time interface check
var _ execute.Executor = (*MockExecutor)(nil)
var _ execute.Block = (*MockBlock)(nil)
var _ execute.BlockState = (*MockBlockState)(nil)

// MockDispatcher implements execute.Dispatcher for testing.
type MockDispatcher struct {
	mu        sync.Mutex
	submitted []*DispatchedEnvelope
	sendCh    chan error
}

// DispatchedEnvelope records a submitted envelope.
type DispatchedEnvelope struct {
	Destination string
	Envelope    *messaging.Envelope
}

// NewMockDispatcher creates a new mock dispatcher.
func NewMockDispatcher() *MockDispatcher {
	return &MockDispatcher{
		submitted: make([]*DispatchedEnvelope, 0),
		sendCh:    make(chan error, 1),
	}
}

// Submit adds an envelope to the queue.
func (d *MockDispatcher) Submit(ctx context.Context, dest *url.URL, envelope *messaging.Envelope) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.submitted = append(d.submitted, &DispatchedEnvelope{
		Destination: dest.String(),
		Envelope:    envelope,
	})
	return nil
}

// Send submits the queued transactions.
func (d *MockDispatcher) Send(ctx context.Context) <-chan error {
	ch := make(chan error, 1)
	ch <- nil
	close(ch)
	return ch
}

// Close closes the dispatcher.
func (d *MockDispatcher) Close() {
	// No-op
}

// GetSubmitted returns all submitted envelopes.
func (d *MockDispatcher) GetSubmitted() []*DispatchedEnvelope {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]*DispatchedEnvelope, len(d.submitted))
	copy(result, d.submitted)
	return result
}

// SubmittedCount returns the number of submitted envelopes.
func (d *MockDispatcher) SubmittedCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.submitted)
}

// Clear clears all submitted envelopes.
func (d *MockDispatcher) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.submitted = nil
}

// MockCommitInfo creates a mock commit info for testing.
func MockCommitInfo(round int64, votes int) *abcitypes.CommitInfo {
	voteInfos := make([]abcitypes.VoteInfo, votes)
	for i := 0; i < votes; i++ {
		voteInfos[i] = abcitypes.VoteInfo{
			Validator: abcitypes.Validator{
				Address: []byte{byte(i)},
				Power:   1,
			},
			BlockIdFlag: tmtypes.BlockIDFlagCommit,
		}
	}
	return &abcitypes.CommitInfo{
		Round: int32(round),
		Votes: voteInfos,
	}
}

// MockBlockParams creates mock block parameters.
func MockBlockParams(index uint64) execute.BlockParams {
	return execute.BlockParams{
		Context:  context.Background(),
		IsLeader: true,
		Index:    index,
		Time:     time.Now(),
	}
}

// MockTransactionStatus creates a mock transaction status.
func MockTransactionStatus(code acmerrors.Status) *protocol.TransactionStatus {
	return &protocol.TransactionStatus{
		Code: code,
	}
}

// ErrMockExecutorFailed is returned when a mock executor operation fails.
var ErrMockExecutorFailed = errors.New("mock executor failed")
