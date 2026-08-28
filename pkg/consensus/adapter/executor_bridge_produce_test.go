// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package adapter

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ProduceBlock was at 0% coverage — the layer where committed transactions
// died invisibly for hours during the 2026-08-20 campaign (per-transaction
// status errors were Debug-only until f15944146). These tests pin the
// batch→envelope→executor contract with a recording fake executor (#4118,
// #4116).

// fakeExec records every envelope processed, in order, and lets tests inject
// failures at each stage of Begin → Process → Close → Hash → Commit.
type fakeExec struct {
	mu        sync.Mutex
	processed []*messaging.Envelope
	statuses  []*protocol.TransactionStatus // returned from every Process

	failBegin, failProcess, failClose, failHash, failCommit bool
	discarded                                               bool

	major   uint64
	majorOK bool

	validateStatuses []*protocol.TransactionStatus
	validateErr      error
}

func (f *fakeExec) EnableTimers()                     {}
func (f *fakeExec) StoreBlockTimers(*logging.DataSet) {}
func (f *fakeExec) LastBlock() (*execute.BlockParams, [32]byte, error) {
	return &execute.BlockParams{Index: 7}, [32]byte{7}, nil
}
func (f *fakeExec) Init([]*execute.ValidatorUpdate) ([]*execute.ValidatorUpdate, error) {
	return nil, nil
}
func (f *fakeExec) Validate(env *messaging.Envelope, _ bool) ([]*protocol.TransactionStatus, error) {
	return f.validateStatuses, f.validateErr
}
func (f *fakeExec) Begin(params execute.BlockParams) (execute.Block, error) {
	if f.failBegin {
		return nil, fmt.Errorf("begin refused")
	}
	return &fakeBlock{f: f, params: params}, nil
}

type fakeBlock struct {
	f      *fakeExec
	params execute.BlockParams
}

func (b *fakeBlock) Params() execute.BlockParams { return b.params }
func (b *fakeBlock) Process(env *messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	if b.f.failProcess {
		return nil, fmt.Errorf("process refused")
	}
	b.f.mu.Lock()
	b.f.processed = append(b.f.processed, env)
	b.f.mu.Unlock()
	return b.f.statuses, nil
}
func (b *fakeBlock) Close() (execute.BlockState, error) {
	if b.f.failClose {
		return nil, fmt.Errorf("close refused")
	}
	return &fakeState{f: b.f, params: b.params}, nil
}

type fakeState struct {
	f      *fakeExec
	params execute.BlockParams
}

func (s *fakeState) Params() execute.BlockParams { return s.params }
func (s *fakeState) IsEmpty() bool               { return len(s.f.processed) == 0 }
func (s *fakeState) DidCompleteMajorBlock() (uint64, time.Time, bool) {
	return s.f.major, time.Unix(1, 0), s.f.majorOK
}
func (s *fakeState) DidUpdateValidators() ([]*execute.ValidatorUpdate, bool) { return nil, false }
func (s *fakeState) ChangeSet() record.Record                                { return nil }

// Hash is a digest over the exact processed order: two nodes that execute
// the same batches in the same order get the same hash, and ANY reordering
// changes it — the #4054 determinism property in miniature.
func (s *fakeState) Hash() ([32]byte, error) {
	if s.f.failHash {
		return [32]byte{}, fmt.Errorf("hash refused")
	}
	h := sha256.New()
	s.f.mu.Lock()
	for _, env := range s.f.processed {
		b, err := env.MarshalBinary()
		if err != nil {
			s.f.mu.Unlock()
			return [32]byte{}, err
		}
		h.Write(b)
	}
	s.f.mu.Unlock()
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out, nil
}
func (s *fakeState) Commit() error {
	if s.f.failCommit {
		return fmt.Errorf("commit refused")
	}
	return nil
}
func (s *fakeState) Discard() { s.f.discarded = true }

// envBytes returns a valid marshaled envelope whose content is unique per n.
func envBytes(t *testing.T, n uint64) []byte {
	t.Helper()
	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.PartitionUrl("bvn1").JoinPath("ledger")
	txn.Body = &protocol.SyntheticDepositCredits{Amount: n}
	env := &messaging.Envelope{Messages: []messaging.Message{
		&messaging.SequencedMessage{
			Message:     &messaging.TransactionMessage{Transaction: txn},
			Source:      protocol.PartitionUrl("bvn2"),
			Destination: protocol.PartitionUrl("bvn1"),
			Number:      n,
		},
	}}
	b, err := env.MarshalBinary()
	require.NoError(t, err)
	return b
}

func newBridge(t *testing.T, f *fakeExec) *ExecutorBridge {
	t.Helper()
	b, err := NewExecutorBridge(ExecutorBridgeConfig{Executor: f, PartitionID: "bvn1"})
	require.NoError(t, err)
	return b
}

// TestProduceBlock_CanonicalOrderIsDeterministic: batches execute in the
// certificate's canonical payload order, transactions in batch order — and
// two independent executors given the same input produce the same hash, while
// a reordering produces a different one (#4054: iterating a map here diverged
// chain entries and BPT roots across validators).
func TestProduceBlock_CanonicalOrderIsDeterministic(t *testing.T) {
	batches := []*types.Batch{
		types.NewBatch([][]byte{envBytes(t, 1), envBytes(t, 2)}),
		types.NewBatch([][]byte{envBytes(t, 3)}),
		types.NewBatch([][]byte{envBytes(t, 4), envBytes(t, 5)}),
	}
	params := BlockParams{Index: 8, Time: time.Unix(100, 0), Batches: batches}

	produce := func(batches []*types.Batch) ([32]byte, *fakeExec) {
		f := new(fakeExec)
		bridge := newBridge(t, f)
		p := params
		p.Batches = batches
		hash, err := bridge.ProduceBlock(context.Background(), p)
		require.NoError(t, err)
		return hash, f
	}

	hash1, f1 := produce(batches)
	require.Len(t, f1.processed, 5, "every transaction of every batch must execute")
	for i, env := range f1.processed {
		seq := env.Messages[0].(*messaging.SequencedMessage)
		require.Equal(t, uint64(i+1), seq.Number,
			"transactions must execute in the certificate's canonical payload order")
	}

	// Same input on a fresh executor: same hash.
	hash2, _ := produce(batches)
	require.Equal(t, hash1, hash2, "identical batches in identical order must produce identical state")

	// Reordered batches: different hash — the divergence #4054 guards against.
	reordered := []*types.Batch{batches[2], batches[0], batches[1]}
	hash3, _ := produce(reordered)
	require.NotEqual(t, hash1, hash3, "a different execution order must be visible in the state hash")
}

// TestProduceBlock_NilBatchFailsTheBlock: CollectBatches guarantees a
// complete batch set before a block is produced (9630ea564), so a nil batch
// here means that invariant broke upstream. Executing the rest anyway is
// exactly the silent-divergence bug of #4116/#4119 — six nodes at one block
// index, six state hashes — so the block must fail instead. This test
// originally pinned the OLD skip behavior as if it were correct; the
// stress-test divergence proved otherwise, which is the assumption-ledger
// methodology working as designed.
func TestProduceBlock_NilBatchFailsTheBlock(t *testing.T) {
	f := new(fakeExec)
	bridge := newBridge(t, f)

	_, err := bridge.ProduceBlock(context.Background(), BlockParams{
		Index:   8,
		Time:    time.Unix(100, 0),
		Batches: []*types.Batch{nil, types.NewBatch([][]byte{envBytes(t, 1)})},
	})
	require.Error(t, err, "a certificate must never execute without one of its batches")
}

// TestProduceBlock_UnmarshalableTransactionSkipped: garbage inside a batch is
// skipped without poisoning the rest of the batch or failing the block.
func TestProduceBlock_UnmarshalableTransactionSkipped(t *testing.T) {
	f := new(fakeExec)
	bridge := newBridge(t, f)

	_, err := bridge.ProduceBlock(context.Background(), BlockParams{
		Index: 8,
		Time:  time.Unix(100, 0),
		Batches: []*types.Batch{types.NewBatch([][]byte{
			envBytes(t, 1),
			[]byte("not an envelope"),
			envBytes(t, 2),
		})},
	})
	require.NoError(t, err)
	require.Len(t, f.processed, 2, "valid transactions around the garbage must still execute")
}

// TestProduceBlock_StatusErrorsSurfaceAtWarn pins the f15944146 logging fix:
// a per-transaction status error is the ONLY trace a committed message leaves
// when the executor rejects it, and it must surface at Warn — at Debug an
// entire class of silent loss (#4111's vanishing anchor signatures) was
// invisible.
func TestProduceBlock_StatusErrorsSurfaceAtWarn(t *testing.T) {
	var mu sync.Mutex
	var warns []slog.Record
	old := slog.Default()
	slog.SetDefault(slog.New(recordHandler{f: func(r slog.Record) {
		if r.Level >= slog.LevelWarn {
			mu.Lock()
			warns = append(warns, r)
			mu.Unlock()
		}
	}}))
	defer slog.SetDefault(old)

	f := new(fakeExec)
	f.statuses = []*protocol.TransactionStatus{{
		TxID:  protocol.PartitionUrl("bvn1").WithTxID([32]byte{1}),
		Code:  errors.NotFound,
		Error: errors.NotFound.With("missing principal"),
	}}
	bridge := newBridge(t, f)

	_, err := bridge.ProduceBlock(context.Background(), BlockParams{
		Index:   8,
		Time:    time.Unix(100, 0),
		Batches: []*types.Batch{types.NewBatch([][]byte{envBytes(t, 1)})},
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, r := range warns {
		if r.Message == "Transaction failed" {
			found = true
		}
	}
	require.True(t, found, "a rejected transaction's status error must surface at Warn, not Debug")
}

// recordHandler adapts a function into a slog.Handler.
type recordHandler struct{ f func(slog.Record) }

func (h recordHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h recordHandler) Handle(_ context.Context, r slog.Record) error {
	h.f(r)
	return nil
}
func (h recordHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h recordHandler) WithGroup(string) slog.Handler      { return h }

// TestProduceBlock_FailurePropagation: a failure at each stage — begin,
// close, hash, commit — fails the block; a hash failure discards the state.
func TestProduceBlock_FailurePropagation(t *testing.T) {
	params := BlockParams{
		Index:   8,
		Time:    time.Unix(100, 0),
		Batches: []*types.Batch{types.NewBatch([][]byte{envBytes(t, 1)})},
	}
	cases := []struct {
		name    string
		set     func(*fakeExec)
		discard bool
	}{
		{"begin", func(f *fakeExec) { f.failBegin = true }, false},
		{"close", func(f *fakeExec) { f.failClose = true }, false},
		{"hash", func(f *fakeExec) { f.failHash = true }, true},
		{"commit", func(f *fakeExec) { f.failCommit = true }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := new(fakeExec)
			c.set(f)
			bridge := newBridge(t, f)
			_, err := bridge.ProduceBlock(context.Background(), params)
			require.Error(t, err)
			require.Equal(t, c.discard, f.discarded,
				"a state that cannot be hashed must be discarded, not leaked")
		})
	}
}

// TestProduceBlock_UpdatesTracking: a produced block updates LastBlock,
// StateHash, and — when the block closed a major block — LastMajorBlock.
func TestProduceBlock_UpdatesTracking(t *testing.T) {
	f := new(fakeExec)
	f.major, f.majorOK = 12, true
	bridge := newBridge(t, f)

	// Seeded from the executor's persisted state.
	idx, _, err := bridge.LastBlock()
	require.NoError(t, err)
	require.Equal(t, uint64(7), idx, "the bridge must seed its tracking from the executor's last block")

	hash, err := bridge.ProduceBlock(context.Background(), BlockParams{
		Index:   8,
		Time:    time.Unix(100, 0),
		Batches: []*types.Batch{types.NewBatch([][]byte{envBytes(t, 1)})},
	})
	require.NoError(t, err)

	idx, got, err := bridge.LastBlock()
	require.NoError(t, err)
	require.Equal(t, uint64(8), idx)
	require.Equal(t, hash, got)
	require.Equal(t, hash, bridge.StateHash())

	major, _, ok := bridge.LastMajorBlock()
	require.True(t, ok)
	require.Equal(t, uint64(12), major)
}

// TestValidateTransaction: unmarshalable bytes are refused, executor status
// errors surface, and clean transactions pass.
func TestValidateTransaction(t *testing.T) {
	f := new(fakeExec)
	bridge := newBridge(t, f)

	require.Error(t, bridge.ValidateTransaction([]byte("junk")),
		"unmarshalable bytes must be refused before reaching the executor")

	f.validateStatuses = []*protocol.TransactionStatus{{
		Error: errors.BadRequest.With("no"),
	}}
	require.Error(t, bridge.ValidateTransaction(envBytes(t, 1)),
		"an executor status error must fail validation")

	f.validateStatuses = nil
	require.NoError(t, bridge.ValidateTransaction(envBytes(t, 1)))
}

// TestSetValidators_FiresExactlyOnChange: the change handler fires when the
// set or the version changes, and never when nothing changed — a handler that
// fires spuriously churns the committee epoch, one that misses a
// version-only bump desynchronizes epochs across nodes (see the comment in
// updateValidatorsFromGlobals).
func TestSetValidators_FiresExactlyOnChange(t *testing.T) {
	f := new(fakeExec)
	bridge := newBridge(t, f)

	var mu sync.Mutex
	fired := 0
	bridge.OnValidatorSetChange(func([]ValidatorInfo, uint64) {
		mu.Lock()
		fired++
		mu.Unlock()
	})
	count := func() int { mu.Lock(); defer mu.Unlock(); return fired }

	set1 := []ValidatorInfo{{PublicKey: [32]byte{1}, Stake: 1, Active: true}}
	bridge.SetValidators(set1, 1)
	require.Equal(t, 1, count(), "a new set must fire the handler")

	bridge.SetValidators(set1, 1)
	require.Equal(t, 1, count(), "an identical set and version must NOT fire the handler")

	bridge.SetValidators(set1, 2)
	require.Equal(t, 2, count(), "a version-only bump MUST fire — epochs track the network version")

	set2 := []ValidatorInfo{{PublicKey: [32]byte{1}, Stake: 1, Active: true}, {PublicKey: [32]byte{2}, Stake: 1, Active: true}}
	bridge.SetValidators(set2, 2)
	require.Equal(t, 3, count(), "a set change must fire the handler")

	// Validators() returns a copy — mutating it must not affect the bridge.
	got := bridge.Validators()
	require.Len(t, got, 2)
	got[0].PublicKey = [32]byte{99}
	require.Equal(t, [32]byte{1}, bridge.Validators()[0].PublicKey,
		"Validators must return a copy, not a live reference")
}
