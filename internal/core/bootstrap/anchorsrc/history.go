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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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
// **The receipt is held to the bpt chain.** Everything a partition anchors
// hangs under its root chain, so a receipt that merely starts at root and ends
// at a signed root chain anchor says root is SOMETHING the partition recorded
// -- a transaction hash has one too (#4301). The anchor signs the root chain's
// height, and the height decides which step of the receipt is a root chain
// entry (splitAtLeaf). That entry must be the one the bpt chain's index
// records as the bpt chain's anchor, and the steps below it must be the
// receipt of the bpt entry the peer names and of no other node.
//
// q reaches the producer's peers, never this node (#4303). servedAt is the
// block the peer said the state was served at, zero if it did not say.
//
// False with no error is a wait: the entry is not on the chain yet, or no
// verified anchor reaches it yet. An error is a root this history will not
// prove as things stand, and what was fetched at it is fetched again: no peer
// answered, the receipt is not the bpt chain's or does not end at a verified
// anchor, or the history has PASSED the root -- an anchor of a later block is
// verified, so the root would be recorded and reachable if it were ever this
// partition's, and it is not.
func (s *Source) ProveRoot(ctx context.Context, q api.Querier, root [32]byte, servedAt uint64) (bool, error) {
	if q == nil {
		return false, errors.BadRequest.With("anchorsrc.ProveRoot: a querier is required")
	}
	if root == ([32]byte{}) {
		return false, errors.BadRequest.With("there is no root to prove: the state was served without a receipt")
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

	wait := func() (bool, error) {
		if servedAt != 0 && latest.block > servedAt {
			return false, errors.NotFound.WithFormat(
				"the history has passed %x: it was served at block %d, the anchor of block %d is verified, and it is not proven",
				root[:4], servedAt, latest.block)
		}
		return false, nil
	}

	q2 := api.Querier2{Querier: q}
	ledger := s.Producer.JoinPath(protocol.Ledger)
	rec, err := q2.QueryChainEntry(ctx, ledger, &api.ChainQuery{
		Name:           "bpt",
		Entry:          root[:],
		IncludeReceipt: &api.ReceiptOptions{ForHeight: latest.index},
	})
	switch {
	case err == nil:
	case errors.Is(err, errors.NotFound):
		return wait() // Not recorded yet: the next block records it
	default:
		// Either the entry is anchored after the latest verified anchor, which
		// is a wait, or the peers failed, which is not. Asking for the entry
		// without its receipt tells them apart.
		_, err2 := q2.QueryChainEntry(ctx, ledger, &api.ChainQuery{Name: "bpt", Entry: root[:]})
		if err2 == nil || errors.Is(err2, errors.NotFound) {
			return wait()
		}
		return false, errors.UnknownError.WithFormat("no peer answered for bpt entry %x: %w", root[:4], err)
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
	signed, ok := s.anchored(r.Anchor)
	if !ok {
		return false, errors.Conflict.WithFormat(
			"the receipt for bpt entry %x ends at %x, which no verified anchor carries", root[:4], r.Anchor)
	}

	// Which step is the root chain's entry is the signed height's to say.
	step, _, leaf, ok := splitAtLeaf(r, signed.index+1)
	if !ok {
		return false, errors.Conflict.WithFormat(
			"no step of the receipt for bpt entry %x is an entry of a root chain of %d", root[:4], signed.index+1)
	}
	indexed, err := bptAnchorOf(ctx, q2, ledger, rec.Index)
	if err != nil {
		return false, errors.UnknownError.WithFormat("find where the bpt chain anchored entry %d: %w", rec.Index, err)
	}
	if indexed.Anchor != leaf {
		return false, errors.Conflict.WithFormat(
			"the receipt for %x enters the root chain at entry %d, and the bpt chain's anchor is entry %d: it is not a bpt entry's",
			root[:4], leaf, indexed.Anchor)
	}
	want, ok := leafPattern(rec.Index, indexed.Source+1)
	if !ok || len(want) != step {
		return false, errors.Conflict.WithFormat(
			"the receipt for %x is not that of entry %d of a bpt chain of %d", root[:4], rec.Index, indexed.Source+1)
	}
	for i, right := range want {
		if r.Entries[i].Right != right {
			return false, errors.Conflict.WithFormat(
				"the receipt for %x is not that of entry %d of a bpt chain of %d", root[:4], rec.Index, indexed.Source+1)
		}
	}
	return true, nil
}

// bptIndexPages bounds how far back bptAnchorOf looks. A root being proven is
// one a peer served as current a moment ago, so its anchor is near the end.
const bptIndexPages = 50

// bptAnchorOf asks the producer's peers where the bpt chain was anchored with
// the given entry in it: the first entry of the bpt chain's index whose source
// is at or past it. It reads the index from its end, a page at a time.
func bptAnchorOf(ctx context.Context, q api.Querier2, ledger *url.URL, entry uint64) (*protocol.IndexEntry, error) {
	var found *protocol.IndexEntry
	expand := true
	for page := uint64(0); page < bptIndexPages; page++ {
		count := uint64(bptIndexPageSize)
		r, err := q.QueryChainEntries(ctx, ledger, &api.ChainQuery{
			Name:  "bpt-index",
			Range: &api.RangeOptions{Start: page * bptIndexPageSize, Count: &count, FromEnd: true, Expand: &expand},
		})
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		if len(r.Records) == 0 {
			break
		}
		// Whatever order the page came in, the earliest entry at or past the
		// one asked for is the answer.
		below := false
		for _, rec := range r.Records {
			v, ok := rec.Value.(*api.IndexEntryRecord)
			if !ok || v.Value == nil {
				return nil, errors.Conflict.WithFormat("entry %d of the bpt chain's index is not an index entry", rec.Index)
			}
			switch {
			case v.Value.Source < entry:
				below = true
			case found == nil || v.Value.Source < found.Source:
				found = v.Value
			}
		}
		if below || uint64(len(r.Records)) < count {
			break
		}
	}
	if found == nil {
		return nil, errors.NotFound.WithFormat("the bpt chain's index records no anchor with entry %d in it", entry)
	}
	return found, nil
}

const bptIndexPageSize = 100

// anchored returns the verified anchor that carries a root chain anchor.
func (s *Source) anchored(anchor []byte) (rootChainAnchor, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, h := range s.history {
		if bytes.Equal(h.anchor[:], anchor) {
			return h, true
		}
	}
	return rootChainAnchor{}, false
}
