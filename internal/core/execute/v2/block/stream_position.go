// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// streamPosition is where one stream stands at the START of the block: how far
// it has been delivered, how far it has been received, and which numbers in
// between are staged — received, held, waiting for the gap ahead of them to
// close.
//
// Read ONCE per block, through Block.positionOf, and shared by everything that
// asks. That is not only tidiness. updateLedger reads the ledger with GetAs
// inside each message's own child batch, and a child does not share its
// parent's value, so every read deep copies the whole ledger — every stream,
// every staged entry. Per message that is O(total backlog), and draining n
// costs O(n^2). See TestSequenceLedgerCostIsPerRead.
//
// This type is the reader that replaces those. The executor's per-message read
// does not go away until staging owns the ledger write (#4169 step 7); this
// step builds the thing that will replace it and puts the position arithmetic
// in one place.
type streamPosition struct {
	stream    stream
	delivered uint64
	received  uint64

	// staged is indexed by offset from delivered: staged[i] is number
	// delivered+1+i. A nil entry is a number we know was received — because
	// something past it arrived — but whose message we do not hold.
	staged []*url.TxID
}

// next is the number this stream is waiting for.
func (p *streamPosition) next() uint64 { return p.delivered + 1 }

// idOf returns the staged message ID for a sequence number, if we hold one.
//
// This is PartitionSyntheticLedger.Get restated, and it is restated rather than
// called so the offset arithmetic — index = n - delivered - 1 — lives in one
// place. Getting it wrong reads a neighbouring stream position and is the
// failure the positional Pending array invites.
func (p *streamPosition) idOf(n uint64) (*url.TxID, bool) {
	if n <= p.delivered || n > p.received {
		return nil, false
	}
	i := n - p.delivered - 1
	if i >= uint64(len(p.staged)) {
		return nil, false // received is ahead of the window we actually hold
	}
	id := p.staged[i]
	return id, id != nil
}

// has reports whether we hold a staged message for this number.
func (p *streamPosition) has(n uint64) bool {
	_, ok := p.idOf(n)
	return ok
}

// positionOf returns where a stream stands, loading it at most once per block.
func (b *Block) positionOf(s stream) (*streamPosition, error) {
	if !s.ok() {
		return nil, errors.InternalError.With("not a stream")
	}
	key := s.ledger.String() + "|" + s.source.String()
	if p, ok := b.positions[key]; ok {
		return p, nil
	}

	var ledger protocol.SequenceLedger
	err := b.Batch.Account(s.ledger).Main().GetAs(&ledger)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load %v: %w", s.ledger, err)
	}
	part := ledger.Partition(s.source)

	p := &streamPosition{
		stream:    s,
		delivered: part.Delivered,
		received:  part.Received,
		staged:    part.Pending,
	}
	if b.positions == nil {
		b.positions = map[string]*streamPosition{}
	}
	b.positions[key] = p
	return p, nil
}
