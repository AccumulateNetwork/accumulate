// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
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

	// cache collects what this block produces for the producer's
	// synthetic/anchor cache; it commits with the block. cacheBlock is what
	// the block's proofs are built from, filled at production and completed
	// at close (healing spec, "The cache").
	cache      *synthcache.Txn
	cacheBlock *synthcache.Block

	// staging is this block's view of the partition's staging: what is held,
	// proven and waiting, plus what this block adds; it commits with the
	// block (executor spec, "Sync").
	staging *execute.StagingTxn

	// proofsValidatedThrough is how many of State.ReceivedAnchors anchor
	// staging has already used to decide waiting proofs this block.
	proofsValidatedThrough int

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

	// positions holds where each stream stands, keyed by ledger and source:
	// the block's working copy of each stream's ledger entry, read once,
	// advanced as the block executes, written back at Close (#4169 step 7).
	//
	// Behind a POINTER because Block is copied by value into closedBlock, and
	// the cache carries a mutex — a cache miss writes it (see positionOf).
	// Copying a mutex is a bug vet reports, and it would also give the copy a
	// second, unrelated lock.
	positions *positionCache

	// staged is the execution order staging settled for this block (#4169
	// step 5). Shadow only: nothing consults it to decide anything.
	staged *executionOrder

	// dnAnchorsAtStart is the directory anchor chain's height when the block
	// began. A synthetic whose proving anchor sits at or past it was admitted
	// by an anchor applied in THIS block (#4169 step 0c) — the case two-round
	// staging exists for, and the count that says whether it is worth having.
	dnAnchorsAtStart int64
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
		// Nothing executed, so nothing is written — but what the block
		// received and holds in staging stays held, and so does what it
		// noted for the cache. Staging is memory fed by consensus; a block
		// that only collected is not a block that saw nothing (executor
		// spec, "Sync").
		s.Batch.Discard()
		s.cache.Commit()
		s.staging.Commit()
		return nil
	}

	err := s.Executor.EventBus.Publish(execute.WillCommitBlock{
		Block: s,
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = s.Batch.Commit()
	if err != nil {
		return err
	}
	// The cache and staging commit after the store: an entry is in the cache
	// only once the chain has it, and staging releases what the block
	// delivered only once the delivery is durable.
	s.cache.Commit()
	s.staging.Commit()
	return nil
}

func (s *closedBlock) Discard() {
	s.Batch.Discard()
	s.cache.Discard()
	s.staging.Discard()
}
