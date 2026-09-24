// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"log/slog"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// LastExecutedBlock is the block this node's state is, or zero when it has
// executed none: what says whether a node is starting from genesis or coming
// back to a network that has moved on (executor spec, "Sync").
//
// IT IS READ FROM A RECORD NO PULL WRITES.
//
// It used to read `<partition>/ledger`, and that is an ACCOUNT — one of the
// accounts the join's pull fetches from a peer and settles into this store.
// The live log of acc-bvn1-val1 names them in the held set:
//
//	Pulled accounts were given up on unanchored: ... first=acc://bvn-BVN1.acme/ledger
//	Pulled accounts were given up on unanchored: ... first=acc://dn.acme/ledger
//
// So what this read at start-up was whatever the previous process's pull left
// behind, and a half-finished pull could make a node that has executed a
// week of blocks look like one that must join from somewhere else (#4344).
//
// The executor writes SystemData(partition).ExecutedBlock with every block it
// commits (block_end.go). SystemData is not an account, so no pull reaches it.
//
// THE FALLBACK. A store written before that record existed does not have it,
// and reading zero there would tell a node that has been running for a week
// that it is the first node of a new network — which executes from genesis
// without asking anyone. So an absent record falls back to the ledger, ONCE,
// and the number is written into the record before anything else runs. From
// that moment the pull cannot move it. The one start that reads the ledger is
// the first start after the upgrade, and it is no worse off than every start
// before it was.
func LastExecutedBlock(db database.Beginner, partition string) (uint64, error) {
	batch := db.Begin(false)
	n, err := batch.SystemData(partition).ExecutedBlock().Get()
	switch {
	case err == nil && n > 0:
		batch.Discard()
		return n, nil
	case err != nil && !errors.Is(err, errors.NotFound):
		batch.Discard()
		return 0, errors.UnknownError.WithFormat("read this node's executed block: %w", err)
	}

	// No record: read the ledger once, and seed the record from it.
	var ledger *protocol.SystemLedger
	err = batch.Account(protocol.PartitionUrl(partition).JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
	batch.Discard()
	switch {
	case errors.Is(err, errors.NotFound):
		// No ledger at all: this node has executed nothing, so it is starting
		// from genesis rather than coming back to a network. Nothing to seed.
		return 0, nil
	case err != nil:
		// Anything else is a store this node cannot read. Treating it as
		// "no ledger" would start a node executing from a checkpoint against
		// state it could not read — silently.
		return 0, errors.UnknownError.Wrap(err)
	}

	slog.Info("This node has no record of its own executed block; seeding it from the ledger this once",
		"module", "join", "partition", partition, "block", ledger.Index)
	write := db.Begin(true)
	defer write.Discard()
	err = write.SystemData(partition).ExecutedBlock().Put(ledger.Index)
	if err != nil {
		return 0, errors.UnknownError.WithFormat("seed this node's executed block: %w", err)
	}
	err = write.Commit()
	if err != nil {
		return 0, errors.UnknownError.WithFormat("seed this node's executed block: %w", err)
	}
	return ledger.Index, nil
}
