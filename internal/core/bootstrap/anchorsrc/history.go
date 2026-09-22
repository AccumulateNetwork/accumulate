// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package anchorsrc

import (
	"context"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// ProveRoot reports whether root is a BPT root of the producer: whether it
// EQUALS the StateTreeAnchor of an anchor a quorum of the producer's
// validators signed and this source verified. There is no other proof.
//
// A signed anchor for block N carries the BPT root block N committed
// (crosschain/anchoring.go; pinned by
// TestAnAnchorsStateTreeAnchorIsTheRootOfItsBlock), which is the root a
// peer's state is CURRENT at while its ledger says N. So a pass a peer served
// at block N is proven the moment block N's anchor is verified, a few blocks
// later — the history closes the gap between the last anchor and current
// state, and nothing under a root no anchor signs is trusted (Paul,
// 2026-09-22: "How can anything in the BPT not be proven? The BPT root is part
// of the signed anchor").
//
// servedAt is the block the peer said the state was served at, zero if it did
// not say. It decides only between the two answers that are not "proven".
//
// False with no error is a wait: no verified anchor carries the root yet, and
// the next one may. An error is a root this source will not prove as things
// stand, and what was fetched at it is fetched again: the root is empty (the
// state was served with no receipt), or the history has PASSED it — an anchor
// of a later block is verified, so the anchor of servedAt would be verified
// too if that block had sent one, and it is not this root.
func (s *Source) ProveRoot(ctx context.Context, root [32]byte, servedAt uint64) (bool, error) {
	if root == ([32]byte{}) {
		return false, errors.BadRequest.With("there is no root to prove: the state was served without a receipt")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// What is verified already is proven whether or not the pool can be read
	// this round; the read is for a root the last one did not reach.
	want := strings.ToLower(s.Producer.String())
	var latest uint64
	search := func() bool {
		latest = 0
		for k, r := range s.roots {
			if k.partition != want {
				continue
			}
			if r == root {
				return true
			}
			if k.block > latest {
				latest = k.block
			}
		}
		return false
	}
	if search() {
		return true, nil
	}
	if err := s.readLocked(ctx); err != nil {
		return false, errors.UnknownError.Wrap(err)
	}
	if search() {
		return true, nil
	}
	if servedAt != 0 && latest > servedAt {
		return false, errors.NotFound.WithFormat(
			"the history has passed %x: it was served at block %d, the anchor of block %d is verified, and no verified anchor carries it",
			root[:4], servedAt, latest)
	}
	return false, nil
}
