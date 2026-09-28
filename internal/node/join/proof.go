// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"bytes"
	"context"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A rootChainAt is the root chain position and anchor a signed anchor of a
// block carries.
type rootChainAt struct {
	index  uint64
	anchor [32]byte
}

// maxRootChains bounds how many blocks' root chain anchors are kept.
const maxRootChains = 4096

// observeRootChain records a signed anchor's root chain position and anchor
// (anchorsrc.Collector.OnRootChain).
func (s *PulledState) observeRootChain(block, index uint64, anchor [32]byte) {
	s.rootMu.Lock()
	defer s.rootMu.Unlock()
	if s.rootChains == nil {
		s.rootChains = map[uint64]rootChainAt{}
	}
	s.rootChains[block] = rootChainAt{index, anchor}
	if len(s.rootChains) > 2*maxRootChains {
		var blocks []uint64
		for b := range s.rootChains {
			blocks = append(blocks, b)
		}
		sort.Slice(blocks, func(i, j int) bool { return blocks[i] < blocks[j] })
		for _, b := range blocks[:len(blocks)-maxRootChains] {
			delete(s.rootChains, b)
		}
	}
}

// signedRootChainsAfter is the signed root chain anchors of the blocks after
// v, the nearest first.
func (s *PulledState) signedRootChainsAfter(v uint64) []rootChainAt {
	s.rootMu.Lock()
	defer s.rootMu.Unlock()
	var blocks []uint64
	for b := range s.rootChains {
		if b > v {
			blocks = append(blocks, b)
		}
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i] < blocks[j] })
	out := make([]rootChainAt, len(blocks))
	for i, b := range blocks {
		out[i] = s.rootChains[b]
	}
	return out
}

// rootOfUnanchored is the proven root of a block V that sent no anchor of its
// own, if a later block's signed root chain proves one: the local root, or
// failing that the root a peer says V had. The second is compared with the
// local root and, if it differs, located (Matched); only the first ever
// hands off.
func (s *PulledState) rootOfUnanchored(ctx context.Context, v uint64, local [32]byte) ([32]byte, bool) {
	if s.provenByBptChain(ctx, v, local) {
		return local, true
	}
	for _, src := range s.partitionSources(ctx) {
		rec, err := src.QueryBptBlock(ctx, s.partition, &api.BptBlockQuery{ForHeight: v})
		if err != nil || rec == nil || rec.Block != v {
			continue
		}
		if s.provenByBptChain(ctx, v, rec.BptRoot) {
			return rec.BptRoot, true
		}
	}
	return [32]byte{}, false
}

// provenByBptChain reports whether root is proven a root of the partition by
// a later block's signed anchor, for a block V that sent no anchor of its own
// (executor spec, "Sync", "The algorithm", step 3).
//
// Every block after V that changes anything records the root after V as an
// entry of the partition ledger's bpt chain, and the root chain of any block
// at or after it anchors that chain. A peer's receipt from the root, as an
// entry of that chain, to the root chain anchor a quorum of the partition's
// validators signed for a later block proves the root was the partition's
// state root after some block. For the local root, which block is not the
// peer's word: the root commits to the ledger account in it, whose index is
// the block, and the local ledger names V.
func (s *PulledState) provenByBptChain(ctx context.Context, v uint64, root [32]byte) bool {
	later := s.signedRootChainsAfter(v)
	if len(later) == 0 {
		return false
	}
	ledger := s.partition.JoinPath(protocol.Ledger)
	for _, src := range s.partitionSources(ctx) {
		for _, at := range later {
			r, err := src.QueryChainEntry(ctx, ledger, &api.ChainQuery{
				Name:           "bpt",
				Entry:          root[:],
				IncludeReceipt: &api.ReceiptOptions{ForHeight: at.index},
			})
			if err != nil || r == nil || r.Receipt == nil {
				continue
			}
			rc := &r.Receipt.Receipt
			if bytes.Equal(rc.Start, root[:]) && bytes.Equal(rc.Anchor, at.anchor[:]) && rc.Validate(nil) {
				return true
			}
		}
	}
	return false
}
