// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// queryAccountChainsAt serves an account's chains AS OF an anchored block
// (#4362, completing #4361).
//
// # Why this exists
//
// #4361 made a peer able to serve an account's BODY as of a past block, with a
// receipt terminating at that block's BPT root. It did not make it able to
// serve the rest of the account. An account's leaf is hashed over four things
// together — the main state, the directory list, the chains and the pending
// list (internal/database/observer_prod.go, hashState) — so a caller that
// takes the body at block B and the chains as they stand NOW hashes a value no
// node ever held. Every account whose chains have moved since B is then
// refused by its own receipt, for ever; and the partition's ledger and its
// anchors move every block, which is exactly and only what a restarting node
// needs. That is the whole of why a restart could not converge.
//
// # What it answers with
//
// Each chain's length at the block and the Merkle state at that length. The
// length comes from the chain's INDEX CHAIN, which records (BlockIndex,
// Source) for every block that appended to it, with Source the chain's last
// index in that block — so the length at B is Source + 1 from the newest entry
// at or before B. The state is computed from the chain itself, because a chain
// is append-only and its state at a length is a function of the entries below
// it.
//
// Two chains of the partition's ledger carry no index chain of their own: the
// BPT chain and the block-ledger chain. They are appended once per indexed
// block, in lockstep with the ROOT index chain, which is what
// [indexing.BPTRootAt] relies on to find the root as of a block — so their
// length at B is the root index chain's position for B, plus one.
//
// A chain that did not exist at the block is served with a length of zero,
// which is what it was: an empty chain hashes as an empty chain, and a caller
// building the leaf needs it in the list.
//
// IT REFUSES RATHER THAN APPROXIMATING. A length it cannot resolve, or a state
// the chain cannot produce for that length, is an error and not a fall-back to
// the current head: a fall-back would serve the caller a coherent-looking
// answer that its own receipt rejects, which is the failure this replaces.
func (s *Querier) queryAccountChainsAt(batch *database.Batch, record *database.Account, height uint64) (*api.RecordRange[*api.ChainRecord], error) {
	live, err := record.Chains().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load chains index: %w", err)
	}

	// COPIED BEFORE ANYTHING ELSE READS A CHAIN. Chains().Get() hands back
	// the set's own slice, and resolving a chain's index chain registers it
	// in that set (database.Chain2.Get) -- which inserts into the slice, in
	// sorted order, while this loop is walking it. Walking the live slice
	// dropped the last chain of every account and reported a different one
	// in its place, so an account's leaf could never be reproduced and the
	// pull refused it for holding "a chain the peer serves none of".
	chains := make([]*protocol.ChainMetadata, len(live))
	copy(chains, live)

	// The root index chain's position for the block, for the two chains that
	// have no index of their own. Resolved once; a block this node cannot
	// resolve is the same refusal an account request makes.
	pos, _, err := indexing.ResolveBlockAtOrBefore(s.partition, batch, height)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	r := new(api.RecordRange[*api.ChainRecord])
	r.Total = uint64(len(chains))
	r.Records = make([]*api.ChainRecord, len(chains))
	for i, c := range chains {
		chain, err := record.ChainByName(c.Name)
		if err != nil {
			return nil, errors.InternalError.WithFormat("get chain %s: %w", c.Name, err)
		}
		count, err := chainCountAt(s, record, chain, height, pos)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("chain %s as of block %d: %w", c.Name, height, err)
		}

		cr := new(api.ChainRecord)
		cr.Name = chain.Name()
		cr.Type = chain.Type()
		cr.Count = count
		if count > 0 {
			state, err := chain.Inner().StateAt(int64(count) - 1)
			if err != nil {
				return nil, errors.UnknownError.WithFormat(
					"chain %s held %d entries at block %d and this node cannot produce its state there: %w",
					c.Name, count, height, err)
			}
			if uint64(state.Count) != count {
				return nil, errors.InternalError.WithFormat(
					"chain %s: asked for the state at %d entries and got %d", c.Name, count, state.Count)
			}
			cr.State = state.Pending
		}
		r.Records[i] = cr
	}
	return r, nil
}

// chainCountAt is how many entries a chain held at the end of the given block.
//
// Three cases, and none of them guesses:
//
//   - An ordinary chain has an INDEX CHAIN recording (BlockIndex, Source) for
//     every block that appended to it, Source being the chain's last index in
//     that block. The newest entry at or before the block gives Source + 1.
//   - An index chain — the major-block chain is the one an account carries —
//     has no index of its own, and needs none: its own entries carry the block
//     they stand for, so the search runs over the chain itself and the
//     position it lands on, plus one, is the length.
//   - The partition ledger's BPT chain and block-ledger chain carry no block
//     in their entries. They run in lockstep with the ROOT index chain, one
//     entry per indexed block, which is what [indexing.BPTRootAt] relies on:
//     the root as of the block at root-index position P is BPT chain entry P.
//     That entry is written by the NEXT state-changing block — BPTRootAt says
//     so itself, refusing with "not on the bpt chain yet" — so at the END of
//     that block the chain holds entries 0 to P-1 and its length is P, not
//     P + 1. Measured on a running partition: root-index 126, bpt 125,
//     block-ledger 125.
func chainCountAt(s *Querier, record *database.Account, chain *database.Chain2, height, rootIndexPos uint64) (uint64, error) {
	if record.Url().Equal(s.partition.Ledger()) {
		switch strings.ToLower(chain.Name()) {
		case "bpt", "block-ledger":
			return rootIndexPos, nil
		}
	}

	search := chain
	if chain.Type() != merkle.ChainTypeIndex {
		search = chain.Index()
	}
	c, err := search.Get()
	if err != nil {
		return 0, errors.UnknownError.Wrap(err)
	}
	h := uint64(c.Height())
	if h == 0 {
		// The chain has never been INDEXED, which is not the same as never
		// having been appended to: genesis writes chains without going
		// through the block path that records an index entry -- the
		// partition ledger's own main chain is one, with its single genesis
		// entry. A chain with no indexed movement has not moved in any block
		// this node executed, so what it holds now is what it held at the
		// block.
		head, err := chain.Inner().Head().Get()
		if err != nil {
			return 0, errors.UnknownError.Wrap(err)
		}
		return uint64(head.Count), nil
	}

	// The search starts at the LAST entry. Started anywhere else, a target
	// past every entry runs off the end and is reported as an error rather
	// than as "the whole chain is at or before it", which is the common case
	// here: a chain that has not moved since the block.
	pos, entry, err := indexing.SearchIndexChain(c, h-1,
		indexing.MatchBefore, indexing.SearchIndexChainByBlock(height))
	switch {
	case err == nil:
		if chain.Type() == merkle.ChainTypeIndex {
			return pos + 1, nil
		}
		return entry.Source + 1, nil
	case errors.Is(err, indexing.ErrReachedChainStart):
		// Every index entry is AFTER the block. What the chain held at the
		// block is then whatever was written before the first indexed one,
		// and nothing records that -- so this node cannot say, and saying
		// zero would serve a caller a chain it can never reconcile.
		return 0, errors.IncompleteChain.WithFormat(
			"this node cannot say how many entries %s held at block %d: its index chain starts after that block",
			chain.Name(), height)
	default:
		return 0, errors.UnknownError.Wrap(err)
	}
}
