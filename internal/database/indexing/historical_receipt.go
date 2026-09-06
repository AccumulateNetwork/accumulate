// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
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

	return &HistoricalStateProof{
		Receipt:        full,
		Block:          block,
		HistoricalRoot: root,
		AnchorBound:    false,
		Partition:      partition.PartitionID(),
	}, nil
}
