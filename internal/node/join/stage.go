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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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

	// Partition is the partition joined. With it, the gap check also asks
	// of every inbound stream the pulled ledgers name, whether or not
	// staging holds anything on it: a stream whose every entry above
	// Delivered arrived before this node was listening has nothing in
	// staging, and its gap is the whole of Delivered + 1 to Received.
	Partition *url.URL
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
	// highest a validated hash stands at; Received the highest the proven
	// state says the partition received. The gap is below the largest.
	Held, Validated, Received uint64
}

// StreamName is the stream as source->ledger, without the scheme.
func (g StreamGap) StreamName() string {
	return execute.StreamName(g.Stream)
}

func (g StreamGap) String() string {
	return fmt.Sprintf("%s delivered=%d missing=%d-%d held=%d validated=%d received=%d",
		g.StreamName(), g.Delivered, g.Missing, g.MissingTo, g.Held, g.Validated, g.Received)
}

// HasGap answers every stream that cannot run contiguously from the pulled
// state's Delivered: a number above Delivered, at or below the highest number
// staging holds or has validated, or the proven state's Received, that
// nothing is held for. An empty answer is no gap.
//
// Received is what the partition had received on the stream at the proven
// block (#4412): every number to it was held by the peers, and the entries
// between Delivered and Received are what they will execute. A node that did
// not hold them executes those blocks differently (executor spec, "Sync",
// "The algorithm", step 6; run 20260928T051402Z, where each Directory
// follower missed on the first blocks after its proof, in its anchor ledger,
// its ledger and the ACME issuer).
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

	type stream struct {
		id              execute.StreamID
		sighted, reach  uint64
		stagedDelivered uint64
	}
	var streams []*stream
	byName := map[string]*stream{}
	add := func(id execute.StreamID) *stream {
		k := execute.StreamName(id)
		if st, ok := byName[k]; ok {
			return st
		}
		st := &stream{id: id}
		byName[k] = st
		streams = append(streams, st)
		return st
	}
	for _, st := range tx.Streams() {
		x := add(st.ID)
		x.sighted, x.reach, x.stagedDelivered = st.Sighted, st.Reach, st.Delivered
	}
	if s.Partition != nil {
		for _, name := range []string{protocol.Synthetic, protocol.AnchorPool} {
			ids, err := inboundStreams(batch, s.Partition.JoinPath(name))
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}
			for _, id := range ids {
				add(id)
			}
		}
	}

	var gaps []StreamGap
	for _, st := range streams {
		delivered, received, err := positionFrom(batch, st.id)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		if st.stagedDelivered > delivered {
			delivered = st.stagedDelivered
		}
		through := max(st.sighted, st.reach, received)
		runs := tx.Missing(st.id, delivered, through, 1)
		if len(runs) == 0 {
			continue
		}
		gaps = append(gaps, StreamGap{
			Stream:    st.id,
			Delivered: delivered,
			Missing:   runs[0][0],
			MissingTo: runs[0][1],
			Held:      st.sighted,
			Validated: st.reach,
			Received:  received,
		})
	}
	return gaps, nil
}

// inboundStreams is every stream a sequence ledger of the pulled state names.
func inboundStreams(batch *database.Batch, ledgerUrl *url.URL) ([]execute.StreamID, error) {
	var ledger protocol.SequenceLedger
	switch err := batch.Account(ledgerUrl).Main().GetAs(&ledger); {
	case errors.Is(err, errors.NotFound):
		return nil, nil
	case err != nil:
		return nil, errors.UnknownError.WithFormat("load %v: %w", ledgerUrl, err)
	}
	var out []execute.StreamID
	for _, p := range sequenceOf(ledger) {
		if p != nil && p.Url != nil {
			out = append(out, execute.StreamID{Ledger: ledgerUrl, Source: p.Url})
		}
	}
	return out, nil
}

func sequenceOf(ledger protocol.SequenceLedger) []*protocol.PartitionSyntheticLedger {
	switch l := ledger.(type) {
	case *protocol.SyntheticLedger:
		return l.Sequence
	case *protocol.AnchorLedger:
		return l.Sequence
	}
	return nil
}

// positionFrom is what a stream's ledger in the pulled state says it has
// delivered from its source, and received.
func positionFrom(batch *database.Batch, id execute.StreamID) (delivered, received uint64, _ error) {
	var ledger protocol.SequenceLedger
	switch err := batch.Account(id.Ledger).Main().GetAs(&ledger); {
	case errors.Is(err, errors.NotFound):
		return 0, 0, nil
	case err != nil:
		return 0, 0, errors.UnknownError.WithFormat("load %v: %w", id.Ledger, err)
	}
	p, ok := ledger.FindPartition(id.Source)
	if !ok {
		return 0, 0, nil
	}
	return p.Delivered, p.Received, nil
}
