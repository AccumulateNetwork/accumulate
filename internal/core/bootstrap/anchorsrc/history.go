// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package anchorsrc

import (
	"bytes"
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// rootChainAnchor is what one verified anchor says about its producer's root
// chain: its anchor, taken when the chain's last entry was index.
type rootChainAnchor struct {
	block  uint64
	index  uint64
	anchor [32]byte
}

// recordHistory keeps a verified anchor's root chain anchor, evicting the
// oldest past MaxRoots.
func (s *Source) recordHistory(a rootChainAnchor) {
	for _, h := range s.history {
		if h.anchor == a.anchor {
			return
		}
	}
	s.history = append(s.history, a)
	max := s.MaxRoots
	if max <= 0 {
		max = DefaultMaxRoots
	}
	if len(s.history) > max {
		s.history = s.history[len(s.history)-max:]
	}
}

// ProveRoot reports whether root is a BPT root the producer's history proves:
// an entry of <producer>/ledger's bpt chain, with a receipt from that entry to
// the root chain anchor a verified anchor carries.
//
// Every block records the previous block's BPT root on the bpt chain, and the
// bpt chain is anchored into the root chain every block (block_end.go, #4272).
// A signed anchor carries the root chain's anchor, so the root a peer's state
// is CURRENT at is provable as soon as any anchor after the block that
// recorded it is verified. The accounts a peer serves are its current ones, so
// this is what lets them settle without waiting for an anchor of their own
// block, which most blocks never get.
//
// q reaches the producer's peers, never this node (#4303). False with no error
// is a wait: the entry is not on the chain yet, or no verified anchor reaches
// it yet. A receipt that does not end at a verified anchor is an error.
func (s *Source) ProveRoot(ctx context.Context, q api.Querier, root [32]byte) (bool, error) {
	if q == nil {
		return false, errors.BadRequest.With("anchorsrc.ProveRoot: a querier is required")
	}

	s.mu.Lock()
	if err := s.readLocked(ctx); err != nil {
		s.mu.Unlock()
		return false, errors.UnknownError.Wrap(err)
	}
	if len(s.history) == 0 {
		s.mu.Unlock()
		return false, nil
	}
	latest := s.history[0]
	for _, h := range s.history[1:] {
		if h.index > latest.index {
			latest = h
		}
	}
	s.mu.Unlock()

	rec, err := api.Querier2{Querier: q}.QueryChainEntry(ctx, s.Producer.JoinPath(protocol.Ledger), &api.ChainQuery{
		Name:           "bpt",
		Entry:          root[:],
		IncludeReceipt: &api.ReceiptOptions{ForHeight: latest.index},
	})
	switch {
	case err == nil:
	case errors.Is(err, errors.NotFound):
		return false, nil // Not recorded yet: the next block records it
	default:
		// The entry exists and is anchored after the latest verified anchor,
		// or the peer failed; either way it is asked again.
		return false, nil
	}
	if rec == nil || rec.Receipt == nil {
		return false, errors.Conflict.WithFormat("a peer served the bpt entry %x with no receipt", root[:4])
	}
	r := &rec.Receipt.Receipt
	if !r.Validate(nil) {
		return false, errors.Conflict.WithFormat("the receipt for bpt entry %x does not validate", root[:4])
	}
	if !bytes.Equal(r.Start, root[:]) {
		return false, errors.Conflict.WithFormat("the receipt for bpt entry %x starts at %x", root[:4], r.Start)
	}
	if !s.anchored(r.Anchor) {
		return false, errors.Conflict.WithFormat(
			"the receipt for bpt entry %x ends at %x, which no verified anchor carries", root[:4], r.Anchor)
	}
	return true, nil
}

// anchored reports whether a root chain anchor is one a verified anchor
// carries.
func (s *Source) anchored(anchor []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, h := range s.history {
		if bytes.Equal(h.anchor[:], anchor) {
			return true
		}
	}
	return false
}
