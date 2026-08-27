// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// Block implements [execute.Block].
type Block struct {
	execute.BlockParams
	State    BlockState
	Batch    *database.Batch
	Executor *Executor

	// produced accumulates every delivery's produced messages so they can be
	// sequenced in ONE sorted pass at block end (#4144). Sequencing inline —
	// destLedger.Produced++ per message as each delivery executes — was the
	// only cross-transaction dependency in the execution path, and it bound
	// sequence numbers to delivery order, which becomes shard-scheduling-
	// dependent under parallel execution (#4145).
	produced []*ProducedMessage

	// fatal poisons the block: a shard child-batch commit failed after
	// possibly writing a prefix of its state into the parent (#4149). No
	// further envelopes execute and Close refuses to produce a state hash.
	fatal error

	// positions caches where each stream stands at the start of the block,
	// keyed by ledger and source, so the ledger is read once per stream per
	// block rather than once per message (#4169 step 2).
	positions map[string]*streamPosition

	// seqReady memoizes the readiness verdict for each sequenced message in
	// this block, keyed by the sequenced message's hash, decided by a serial
	// pre-pass before any shard runs (see decideSequencedReadiness).
	//
	// Readiness is `partitionLedger.Delivered+1 == seq.Number`, so it depends
	// on how far the stream has advanced. Deciding it inside execution makes
	// it depend on WHEN a shard happens to run: two messages of one stream
	// arriving in the same block on different shards would disagree about
	// whether the second is next, and nodes at different shard counts would
	// diverge on a state hash. Deciding it once, serially, in arrival order,
	// removes the scheduling from the answer.
	//
	// A message with no entry here falls back to the live check — cascade
	// messages (#4146) are generated during execution and were never seen by
	// the pre-pass, and their behaviour is unchanged.
	seqReady map[[32]byte]bool
}

func (b *Block) Params() execute.BlockParams { return b.BlockParams }

// closedBlock implements [execute.BlockState].
type closedBlock struct {
	Block
	valUp []*execute.ValidatorUpdate
}

func (b *closedBlock) Params() execute.BlockParams { return b.BlockParams }
func (s *closedBlock) ChangeSet() record.Record    { return s.Batch }
func (s *closedBlock) IsEmpty() bool               { return s.State.Empty() }

func (s *closedBlock) DidCompleteMajorBlock() (uint64, time.Time, bool) {
	return s.didOpenMajorBlock()
}

func (s *closedBlock) DidUpdateValidators() ([]*execute.ValidatorUpdate, bool) {
	return s.valUp, len(s.valUp) > 0
}

func (s *closedBlock) Hash() ([32]byte, error) {
	return s.Batch.GetBptRootHash()
}

func (s *closedBlock) Commit() error {
	if s.IsEmpty() {
		s.Discard()
		return nil
	}

	err := s.Executor.EventBus.Publish(execute.WillCommitBlock{
		Block: s,
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return s.Batch.Commit()
}

func (s *closedBlock) Discard() {
	s.Batch.Discard()
}
