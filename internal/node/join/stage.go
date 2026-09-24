// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A Settler is the executor's side of a stage: it brings staging to the
// block the pulled state is.
type Settler interface {
	SettleStagingAt(q uint64) error
}

// ExecutorStage is the Stage a node joins with: the executor settles its
// staging, and the gap check reads that staging against the Delivered the
// pulled state holds.
type ExecutorStage struct {
	Settler
	Staging  *execute.Staging
	Database *database.Database
}

// A StreamGap is one stream that cannot run contiguously from the pulled
// state's Delivered: what the gap check found, as the join logs it and the
// gauge carries it (#4432).
type StreamGap struct {
	Stream execute.StreamID

	// Delivered is what the stream has delivered: the pulled ledger's word,
	// or staging's where staging has released further.
	Delivered uint64

	// Missing through MissingTo is the first run of numbers above Delivered
	// that nothing is held for.
	Missing, MissingTo uint64

	// Held is the highest number staging holds on the stream; Validated the
	// highest a validated hash stands at. The gap is below the larger.
	Held, Validated uint64
}

// StreamName is the stream as source->ledger, without the scheme.
func (g StreamGap) StreamName() string {
	return execute.StreamName(g.Stream)
}

func (g StreamGap) String() string {
	return fmt.Sprintf("%s delivered=%d missing=%d-%d held=%d validated=%d",
		g.StreamName(), g.Delivered, g.Missing, g.MissingTo, g.Held, g.Validated)
}

// HasGap answers every stream staging holds entries on that cannot run
// contiguously from the pulled state's Delivered: a number above Delivered,
// below the highest number held or validated, that nothing is held for. An
// empty answer is no gap.
//
// It does not know which of the staged entries the block carries — a
// collected group has no block number — so it asks the question of
// everything staged, which is everything collected through block and nothing
// after it (Buffer.StageThrough, #4398). Entries from before block are asked
// about too: a hole below what block carries is one it would need filled.
//
// It reads staging and the pulled ledgers, and nothing else: no message's
// status enters the answer.
func (s *ExecutorStage) HasGap(block uint64) ([]StreamGap, error) {
	if s.Staging == nil || s.Database == nil {
		return nil, errors.BadRequest.With("a stage needs staging and a database")
	}
	batch := s.Database.Begin(false)
	defer batch.Discard()
	tx := s.Staging.Begin()
	defer tx.Discard()

	var gaps []StreamGap
	for _, st := range tx.Streams() {
		delivered, err := deliveredFrom(batch, st.ID)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		if st.Delivered > delivered {
			delivered = st.Delivered
		}
		through := st.Sighted
		if st.Reach > through {
			through = st.Reach
		}
		runs := tx.Missing(st.ID, delivered, through, 1)
		if len(runs) == 0 {
			continue
		}
		gaps = append(gaps, StreamGap{
			Stream:    st.ID,
			Delivered: delivered,
			Missing:   runs[0][0],
			MissingTo: runs[0][1],
			Held:      st.Sighted,
			Validated: st.Reach,
		})
	}
	return gaps, nil
}

// deliveredFrom is what a stream's ledger in the pulled state says it has
// delivered from its source.
func deliveredFrom(batch *database.Batch, id execute.StreamID) (uint64, error) {
	var ledger protocol.SequenceLedger
	switch err := batch.Account(id.Ledger).Main().GetAs(&ledger); {
	case errors.Is(err, errors.NotFound):
		return 0, nil
	case err != nil:
		return 0, errors.UnknownError.WithFormat("load %v: %w", id.Ledger, err)
	}
	return ledger.Partition(id.Source).Delivered, nil
}
