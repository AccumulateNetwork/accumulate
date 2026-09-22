// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"bytes"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// HistoricalStateProof is a proof that an account held a particular state at a
// past block, together with what the terminating root is and is not.
type HistoricalStateProof struct {
	// Receipt runs from the account's state hash as of Block to the BPT root
	// as of Block. It validates offline.
	Receipt *merkle.Receipt

	// Block is the block the state is proven as of. It is the last
	// state-changing block at or before the height that was requested, which is
	// exact: a block that changed nothing carries its predecessor's root.
	Block uint64

	// HistoricalRoot is the BPT root as of Block, taken from the ledger's bpt
	// chain. The receipt terminates at it.
	HistoricalRoot [32]byte

	// Partition identifies whose BPT root the receipt terminates at, so a
	// caller knows what to finish the binding against.
	Partition string

	// State is the account's main state AS OF Block — the body the receipt
	// proves, not the body the account holds now. Serving the current body
	// beside a past receipt would hand a caller two things that do not fit
	// together: the body does not hash to the receipt's start, so a pulling
	// node refuses it and a trusting one keeps state no anchor covers.
	State protocol.Account

	// Leaf is the rest of the account's BPT entry as of Block — every chain's
	// head, the directory, the pending sets and, for the ledgers, the events
	// root and delivery queues — so a caller rebuilds the entry the receipt
	// proves from what it was served. It is nil when the node retained the
	// body for Block but not the leaf, which a node that retained before the
	// leaf was does (#4361).
	Leaf *database.RetainedLeaf

	// StartsAtMainState reports whether the receipt starts at a plain hash of
	// the account's main state, rather than at the account's whole BPT entry.
	//
	// It matters because a verifier holds the account state and nothing else. If
	// this is false the verifier cannot compute the receipt's starting point and
	// must take the server's word for it, which is the trust this proof exists to
	// remove. It is true when the node retained the account's state receipt for
	// Block, which requires retention to have been on at the time.
	StartsAtMainState bool
}

// HistoricalAccountStateProof proves what an account held at a past block.
//
//	account state hash -> account BPT entry @ B    the retained state receipt
//	account BPT entry  -> BPT root @ B             BPT membership, retained nodes
//
// # Where it terminates, and why this line differs from main
//
// The proof ENDS at the BPT root as of B. On `main` (AIP-58) it does not: it
// continues through the ledger's bpt chain to the partition's CURRENT root,
// because "an arbitrary block's root is not something a client can check" —
// only the sparse subset of roots that reached a cross-partition anchor is
// covered by a quorum signature.
//
// That reason does not hold here, and the opposite is what the join needs.
//
//   - It is checkable. The root as of B is exactly the `StateTreeAnchor` the
//     partition's anchor for block B carries (internal/core/crosschain/
//     anchoring.go), and a joining node validates that anchor against the
//     operators' key book before it pulls anything (executor.md, "Sync", §1).
//     So the terminus is a value the caller already holds and has verified.
//   - It is bindable anyway. This line anchors the ledger's bpt chain into the
//     root chain (#4272, Kourou), so ANY root on that chain — a past one
//     included — extends to a root-chain anchor and binds to a directory root
//     through ProofService.AnchorReceipt (#4274/#4276). A caller wanting main's
//     reach makes that second call with this proof's HistoricalRoot.
//   - Terminating at the current root is the defect. "A peer serves an account
//     with a receipt running from the account's state hash to that peer's BPT
//     root" cannot settle for an account that changes every block, because no
//     anchor ever covers the root it was served at — which is #4361 itself.
//
// It refuses rather than approximating: outside the retained range, before the
// node's horizon, or for an account that did not exist at the height, it returns
// the corresponding error from [ResolveHistoricalAccountState] and no receipt.
func HistoricalAccountStateProof(partition config.NetworkUrl, batch *database.Batch, account *database.Account, height uint64) (*HistoricalStateProof, error) {
	// Resolve and refuse first. This is what distinguishes "unchanged since"
	// from "older than we keep" — BPT.NodeAt cannot.
	entry, err := ResolveHistoricalAccountState(partition, batch, account, height)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	root, block, err := BPTRootAt(partition, batch, height)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if block != entry.BlockIndex {
		return nil, errors.InternalError.WithFormat(
			"resolution and root lookup disagree: block %d against %d", entry.BlockIndex, block)
	}

	// The account's state hash at that block, proven against the root the
	// ledger recorded for it. GetReceiptAt refuses if the tree reconstructed
	// from retained nodes does not hash to that root.
	full, err := batch.BPT().GetReceiptAt(account.Key(), block, root)
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		// The leaf is not in the tree this node reconstructed for that block.
		// That is NOT proof the account was absent: the reconstruction is
		// checked against the recorded root only AFTER the leaf is found
		// (bpt.GetReceiptAt), so a leaf this node failed to rebuild looks
		// exactly like a leaf that was never there. A requester reads NotFound
		// as a fact about the record and a unanimous one as the network's
		// (join/sources.go), so saying it here would drop an account on the
		// strength of a local failure. It is a capability limit and it says so.
		return nil, errors.IncompleteChain.WithFormat(
			"%v has no leaf in the tree this node can rebuild for block %d", account.Url(), block)
	default:
		return nil, errors.UnknownError.WithFormat("historical membership receipt: %w", err)
	}

	// The body as of that block, and the receipt from its hash to the entry, so
	// a verifier recomputes the starting point from the state it was handed
	// rather than taking the server's word for it.
	startsAtMain := false
	state, body, leaf, err := retainedAccountAt(account, entry.BlockIndex)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if state != nil && bytes.Equal(state.Anchor, full.Start) {
		full, err = state.Combine(full)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("combine state and membership receipts: %w", err)
		}
		startsAtMain = true
	} else if state != nil {
		// The retained receipt does not reach the entry this proof starts at.
		// Under -tags debug that is loud, because in testing it means retention
		// wrote the wrong thing rather than a node that simply cannot help.
		err = debugMismatch(state.Anchor, full.Start)
		if err != nil {
			return nil, err
		}
	}

	// A HISTORICAL ANSWER IS COHERENT OR IT IS A REFUSAL. `main` degrades here
	// to an entry-rooted proof, which is correct on main because main serves
	// the CURRENT body regardless. Here the response carries the body the
	// receipt proves, so there is no such thing as half an answer: without the
	// body at that block there is nothing to put beside the receipt but the
	// body the account holds now, which does not hash to the receipt's start.
	// A caller checking would refuse it and a caller not checking would keep
	// state no anchor covers.
	if !startsAtMain || body == nil {
		return nil, errors.IncompleteChain.WithFormat(
			"%v cannot be served as of block %d: this node retains its BPT entry for that block but not the state behind it",
			account.Url(), block)
	}

	// THE LEAF IS THE ENTRY'S OR IT IS NOT SERVED AT ALL. A leaf that does not
	// rebuild the entry the receipt starts from is a leaf of some other state,
	// and serving it beside a checking receipt hands a puller parts that
	// contradict the proof they came with.
	if leaf != nil {
		rebuilt, err := leaf.EntryHash(state.Start)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		if !bytes.Equal(rebuilt, state.Anchor) {
			err = debugMismatch(rebuilt, state.Anchor)
			if err != nil {
				return nil, err
			}
			return nil, errors.IncompleteChain.WithFormat(
				"%v cannot be served as of block %d: the leaf this node retained for that block does not rebuild its BPT entry",
				account.Url(), block)
		}
	}

	return &HistoricalStateProof{
		Leaf:              leaf,
		Receipt:           full,
		Block:             block,
		HistoricalRoot:    root,
		Partition:         partition.PartitionID(),
		State:             body,
		StartsAtMainState: startsAtMain,
	}, nil
}

// retainedAccountAt returns the account's main state as of the given block and
// the receipt from that state's hash to the account's BPT entry, or nil for
// both when the node cannot produce them.
//
// Retention writes at the blocks where the account CHANGED, and what it writes
// is the state after that block's change, so the record covering block B is the
// newest at or before B — the same rule the BPT uses for the entry itself.
//
// Three cases, and the middle one is the gap worth naming:
//
//   - An entry at or before B: that is the state at B, exactly. The account may
//     have changed since; retention is continuous across the retained range, so
//     it did not change between that entry and B.
//   - NO entry at all: the account has not been written since the retained range
//     began, which is at or before B, so the account has not changed since B and
//     the CURRENT state is the state at B. That is the same rule BPT.NodeAt
//     applies to a node with no later version, and it is why a cold account
//     costs nothing to serve.
//   - Entries, but all of them after B: the account changed inside the window
//     but not at or before B, so the state it held at B is the state it held
//     when the window opened, and that was never written. Nothing is returned;
//     the caller serves an entry-rooted receipt with no body and says so through
//     StartsAtMainState. It affects accounts whose only change in the window is
//     recent, asked about at a block older than that change — not the case a
//     join makes, which asks about the newest anchored block.
func retainedAccountAt(account *database.Account, block uint64) (*merkle.Receipt, protocol.Account, *database.RetainedLeaf, error) {
	blocks, err := account.RetainedStateReceiptBlocks().Get()
	if err != nil {
		return nil, nil, nil, errors.UnknownError.WithFormat("load retained state receipt blocks: %w", err)
	}

	if len(blocks) == 0 {
		// Unchanged since the window opened: the current state is the state at
		// the block, and its receipt is computed rather than remembered.
		state, err := account.Main().Get()
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.NotFound):
			return nil, nil, nil, nil
		default:
			return nil, nil, nil, errors.UnknownError.WithFormat("load main state: %w", err)
		}
		r, err := account.StateTreeReceipt()
		if err != nil {
			return nil, nil, nil, errors.UnknownError.Wrap(err)
		}
		leaf, err := account.LeafState()
		if err != nil {
			return nil, nil, nil, errors.UnknownError.Wrap(err)
		}
		return r, state, leaf, nil
	}

	i := sort.Search(len(blocks), func(i int) bool { return blocks[i] > block })
	if i == 0 {
		return nil, nil, nil, nil // Nothing retained at or before the block
	}

	r, err := account.RetainedStateReceipt(blocks[i-1]).Get()
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return nil, nil, nil, nil
	default:
		return nil, nil, nil, errors.UnknownError.WithFormat("load retained state receipt: %w", err)
	}

	encoded, err := account.RetainedMainState(blocks[i-1]).Get()
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		// The receipt without the body it proves is not something to serve.
		return nil, nil, nil, nil
	default:
		return nil, nil, nil, errors.UnknownError.WithFormat("load retained main state: %w", err)
	}
	if len(encoded) == 0 {
		return nil, nil, nil, nil // Pruned out of the window
	}
	state, err := protocol.UnmarshalAccount(encoded)
	if err != nil {
		return nil, nil, nil, errors.UnknownError.WithFormat("unmarshal retained main state: %w", err)
	}

	// The rest of the leaf, retained with the body. Missing only on a block
	// retained before the leaf was, which is served without it.
	leaf, err := account.RetainedLeaf(blocks[i-1]).Get()
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		leaf = nil
	default:
		return nil, nil, nil, errors.UnknownError.WithFormat("load retained leaf: %w", err)
	}
	return r, state, leaf, nil
}
