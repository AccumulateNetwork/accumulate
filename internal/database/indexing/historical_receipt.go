// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"bytes"
	"crypto/sha256"
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
	// Receipt runs from the account's state hash as of Block to the partition's
	// CURRENT BPT root. It validates offline.
	Receipt *merkle.Receipt

	// Block is the block the state is proven as of. It is the last
	// state-changing block at or before the height that was requested, which is
	// exact: a block that changed nothing carries its predecessor's root.
	Block uint64

	// HistoricalRoot is the BPT root as of Block, taken from the ledger's bpt
	// chain. The receipt passes through it.
	HistoricalRoot [32]byte

	// AnchorBound reports whether the terminating root has itself been carried
	// into an anchor and signed by a quorum, as opposed to merely being this
	// node's current root.
	//
	// It is FALSE here, always, and that is not a placeholder. The receipt
	// terminates at this partition's current BPT root; whether that root has
	// been anchored is a question about the anchor chains, which live on the
	// directory for a BVN. A caller completes the binding there. Reporting the
	// root as signed because it is probably about to be would be the same class
	// of error as answering a historical query with the current root.
	AnchorBound bool

	// Partition identifies whose BPT root the receipt terminates at, so a
	// caller knows which anchor chain to finish the binding against —
	// anchor(<partition>)-bpt on the directory.
	Partition string

	// StartsAtMainState reports whether Receipt begins at a simple hash of the
	// account's main state, as the current-state receipt does, rather than at
	// the whole BPT entry — H(main, secondary, chains, pending).
	//
	// It matters because a verifier holds the account state and nothing else. If
	// this is false the verifier cannot compute the receipt's starting point and
	// must take the server's word for it, which is the trust this proof exists to
	// remove. It is true exactly when State is set: the receipt starts at the
	// hash of State and of nothing else.
	StartsAtMainState bool

	// State is the account's main state AS OF Block - the body the receipt
	// starts at, not the body the account holds now. It is nil when the node
	// cannot produce that body, and then StartsAtMainState is false. Serving the
	// current body beside a past receipt would hand a caller two things that do
	// not fit: the body does not hash to the receipt's start.
	State protocol.Account
}

// HistoricalAccountStateProof proves what an account held at a past block.
//
// The proof is two receipts joined at the historical root:
//
//	account state hash -> historical BPT root     BPT membership, from retained nodes
//	historical root    -> current BPT root        the ledger's bpt chain, bound via
//	                                              the ledger account's BPT entry
//
// The second half is what makes the first half worth anything. A historical root
// on its own is a number this node asserts; bound to the current root it is a
// number the network's own state commits to, and the current root is what gets
// anchored and signed.
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
	// ledger recorded for it
	membership, err := batch.BPT().GetReceiptAt(account.Key(), block, root)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("historical membership receipt: %w", err)
	}

	// That root, proven into the current BPT root
	pos, _, err := ResolveBlockAtOrBefore(partition, batch, height)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	binding, err := batch.Account(partition.Ledger()).ChainEntryReceipt("bpt", int64(pos))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("bind historical root: %w", err)
	}

	if !bytes.Equal(membership.Anchor, binding.Start) {
		return nil, errors.InternalError.WithFormat(
			"the historical root the membership receipt reaches (%x) is not the one being bound (%x)",
			membership.Anchor, binding.Start)
	}

	full, err := membership.Combine(binding)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("combine membership and binding receipts: %w", err)
	}

	// Start at the main state hash where the node can produce the body that
	// hash is of, so a verifier recomputes the starting point from the state it
	// was handed. A body is used only when its receipt reaches exactly the entry
	// this proof starts at AND the body hashes to exactly where that receipt
	// starts: then it is the state at the block, proven, not assumed.
	startsAtMain := false
	var body protocol.Account
	state, retainedBody, err := retainedStateAt(account, entry.BlockIndex)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if state != nil && bytes.Equal(state.Anchor, full.Start) {
		// A receipt retained without its body - retained before bodies were -
		// is not a main-state start a caller can check, so it is not used as
		// one. The current body can still serve if the account has not changed.
		if retainedBody != nil && startsAt(state, retainedBody) {
			full, err = state.Combine(full)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("combine state and membership receipts: %w", err)
			}
			startsAtMain, body = true, retainedBody
		}
	} else if state != nil {
		// The retained receipt does not reach the entry this proof starts at, so
		// it cannot support a main-state start. That is the same condition as
		// retention being off or the debug observer collapsing the components,
		// and it degrades the same way: report StartsAtMainState false and return
		// the entry-rooted proof, which is still correct and still useful.
		// Denying a proof the caller could have used would be the one
		// inconsistent answer among the four.
		//
		// Under -tags debug it is loud instead, because in testing a mismatch
		// means a bug in retention rather than a node that simply cannot help.
		err = debugMismatch(state.Anchor, full.Start)
		if err != nil {
			return nil, err
		}
	}

	// The account as it stands now, when its entry now IS its entry at the
	// block. That is not an assumption about retention: the current receipt
	// runs from the current main state hash to the current entry, and that
	// entry is the one the historical membership receipt starts at, so the
	// current body is the body at the block by the hashes themselves. It is
	// what makes an account that has not changed since the block cost nothing
	// to serve.
	if !startsAtMain {
		current, err := account.StateTreeReceipt()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("current state receipt: %w", err)
		}
		if current != nil && bytes.Equal(current.Anchor, full.Start) {
			now, err := account.Main().Get()
			switch {
			case err == nil:
				if startsAt(current, now) {
					full, err = current.Combine(full)
					if err != nil {
						return nil, errors.UnknownError.WithFormat("combine state and membership receipts: %w", err)
					}
					startsAtMain, body = true, now
				}
			case errors.Is(err, errors.NotFound):
				// No main state, so no body to serve
			default:
				return nil, errors.UnknownError.WithFormat("load main state: %w", err)
			}
		}
	}

	return &HistoricalStateProof{
		Receipt:           full,
		Block:             block,
		HistoricalRoot:    root,
		AnchorBound:       false,
		Partition:         partition.PartitionID(),
		StartsAtMainState: startsAtMain,
		State:             body,
	}, nil
}

// startsAt reports whether the receipt starts at the hash of the body, as the
// BPT hashes a main state: SHA-256 of its binary marshalling.
func startsAt(r *merkle.Receipt, body protocol.Account) bool {
	data, err := body.MarshalBinary()
	if err != nil {
		return false
	}
	h := sha256.Sum256(data)
	return bytes.Equal(r.Start, h[:])
}

// retainedStateAt returns the receipt from the account's main state hash to its
// BPT entry as of the given block, and the main state that receipt starts at.
// Either is nil when the node did not retain it.
//
// Both are retained at the blocks where the account changed, so the one
// covering a block is the newest retained at or before it - the same rule the
// BPT uses for the entry itself.
func retainedStateAt(account *database.Account, block uint64) (*merkle.Receipt, protocol.Account, error) {
	blocks, err := account.RetainedStateReceiptBlocks().Get()
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("load retained state receipt blocks: %w", err)
	}
	if len(blocks) == 0 {
		return nil, nil, nil
	}

	i := sort.Search(len(blocks), func(i int) bool { return blocks[i] > block })
	if i == 0 {
		return nil, nil, nil // Nothing retained at or before the block
	}

	r, err := account.RetainedStateReceipt(blocks[i-1]).Get()
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return nil, nil, nil
	default:
		return nil, nil, errors.UnknownError.WithFormat("load retained state receipt: %w", err)
	}

	encoded, err := account.RetainedMainState(blocks[i-1]).Get()
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return r, nil, nil // Retained before bodies were
	default:
		return nil, nil, errors.UnknownError.WithFormat("load retained main state: %w", err)
	}
	if len(encoded) == 0 {
		return r, nil, nil
	}
	body, err := protocol.UnmarshalAccount(encoded)
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("unmarshal retained main state: %w", err)
	}
	return r, body, nil
}
