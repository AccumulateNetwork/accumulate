// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A historical account state proof (AIP-58) answers "what did this account hold
// at minor block H" with a BPT membership receipt against the BPT root as of H,
// rather than against the current root. This file resolves H to a block this
// node can actually speak to, and refuses — distinguishably — when it cannot.
//
// Refusing is the point. Answering a question about the past with the present
// root would be confidently wrong, which is worse than an error, so nothing
// here ever falls back to the current root.
//
// Retention of superseded BPT state is not implemented. The retained depth is
// therefore zero everywhere today, every historical request is refused with
// [errors.IncompleteChain], and that is a correct, honest node.

// BlockRange is an inclusive range of minor block heights.
type BlockRange struct {
	// Earliest is the lowest block in the range.
	Earliest uint64

	// Latest is the highest block in the range.
	Latest uint64
}

// IsEmpty reports whether the range contains no blocks.
func (r BlockRange) IsEmpty() bool { return r.Earliest > r.Latest }

// Contains reports whether the range contains the given block.
func (r BlockRange) Contains(block uint64) bool {
	return !r.IsEmpty() && block >= r.Earliest && block <= r.Latest
}

// String returns a human-readable form of the range, for error messages.
func (r BlockRange) String() string {
	if r.IsEmpty() {
		return "empty"
	}
	return fmt.Sprintf("[%d, %d]", r.Earliest, r.Latest)
}

// IndexedBlockRange returns the range of minor blocks this node's partition
// ledger has indexed, taken from the first and last entries of the root index
// chain.
//
// Earliest is this node's horizon, not the network's. A node restored from a
// snapshot has no record of anything before the restore point, and no
// incarnation concept exists to say whether an earlier block belonged to the
// same network at all, so a request below Earliest is refused rather than
// resolved forward.
func IndexedBlockRange(partition config.NetworkUrl, batch *database.Batch) (BlockRange, error) {
	rootIndexChain, err := batch.Account(partition.Ledger()).RootChain().Index().Get()
	if err != nil {
		return BlockRange{}, errors.UnknownError.WithFormat("load minor root index chain: %w", err)
	}
	height := rootIndexChain.Height()
	if height == 0 {
		return BlockRange{}, errors.InternalError.With("root index chain is empty")
	}

	first := new(protocol.IndexEntry)
	err = rootIndexChain.EntryAs(0, first)
	if err != nil {
		return BlockRange{}, errors.UnknownError.WithFormat("load minor root index chain entry %d: %w", 0, err)
	}

	last := new(protocol.IndexEntry)
	err = rootIndexChain.EntryAs(height-1, last)
	if err != nil {
		return BlockRange{}, errors.UnknownError.WithFormat("load minor root index chain entry %d: %w", height-1, err)
	}

	return BlockRange{Earliest: first.BlockIndex, Latest: last.BlockIndex}, nil
}

// RetainedBlockRange returns the range of minor blocks for which this node can
// produce a historical BPT membership receipt.
//
// The earliest end is read from what the node ACTUALLY retained, not from its
// configured depth. Raising the depth does not retroactively create history, and
// a node advertising a range it does not hold is worse than one advertising
// nothing — a client would believe it. The two cannot drift because there is
// only one source: the BPT records the horizon as it retains.
//
// An empty range means the node retains no history, which is every node running
// the default configuration.
func RetainedBlockRange(partition config.NetworkUrl, batch *database.Batch) (BlockRange, error) {
	earliest, ok, err := batch.BPT().EarliestRetained()
	if err != nil {
		return BlockRange{}, errors.UnknownError.WithFormat("load earliest retained height: %w", err)
	}
	if !ok {
		return BlockRange{Earliest: 1, Latest: 0}, nil // Empty
	}

	indexed, err := IndexedBlockRange(partition, batch)
	if err != nil {
		return BlockRange{}, errors.UnknownError.Wrap(err)
	}
	if earliest < indexed.Earliest {
		earliest = indexed.Earliest
	}
	return BlockRange{Earliest: earliest, Latest: indexed.Latest}, nil
}

// ResolveBlockAtOrBefore resolves a requested minor block height to the last
// block at or before it that the partition ledger has indexed, and returns that
// root index chain entry together with its position on the index chain.
//
// Resolution moves BACKWARD, and the result is exact rather than approximate.
// A partition indexes only the blocks that changed state; a block that changed
// nothing has the same BPT root as its predecessor. So the last indexed block
// at or before H holds precisely the state as of H — measured, not assumed:
// across every observed block transition, the BPT root changed if and only if
// the ledger's BptChain grew.
//
// Resolving forward would be wrong here, and wrongly in the dangerous
// direction. The state at a later block includes changes that had not happened
// at H, so a forward-resolved receipt would prove a key page's *later* version
// against a signature made under its earlier one — a confident, checkable, and
// false answer. Whether the resolved root can be bound to a quorum signature is
// a separate question from whether it is the right root; this function answers
// only the second.
//
// The position on the index chain is returned because it is the key into the
// ledger's BptChain, which holds the root for that block — see [BPTRootAt].
func ResolveBlockAtOrBefore(partition config.NetworkUrl, batch *database.Batch, height uint64) (uint64, *protocol.IndexEntry, error) {
	if height == 0 {
		return 0, nil, errors.BadRequest.With("cannot resolve block zero: zero means the current state")
	}

	indexed, err := IndexedBlockRange(partition, batch)
	if err != nil {
		return 0, nil, errors.UnknownError.Wrap(err)
	}

	// Below this node's horizon. Do not resolve backward past it — see
	// IndexedBlockRange.
	if height < indexed.Earliest {
		return 0, nil, errors.NotFound.WithFormat(
			"block %d precedes this node's earliest indexed block %d", height, indexed.Earliest)
	}

	// Beyond what this node has indexed. The state at H may well equal the
	// state at the latest indexed block — but this node cannot tell a recent
	// empty block from a block that has not happened, and guessing which would
	// mean answering for a block that may not exist. This is "not yet", not
	// "never".
	if height > indexed.Latest {
		return 0, nil, errors.NotFound.WithFormat(
			"block %d is beyond this node's latest indexed block %d", height, indexed.Latest)
	}

	rootIndexChain, err := batch.Account(partition.Ledger()).RootChain().Index().Get()
	if err != nil {
		return 0, nil, errors.UnknownError.WithFormat("load minor root index chain: %w", err)
	}

	pos, entry, err := SearchIndexChain(rootIndexChain, uint64(rootIndexChain.Height())-1, MatchBefore, SearchIndexChainByBlock(height))
	if err != nil {
		return 0, nil, errors.UnknownError.WithFormat("locate index entry for block %d of the minor root chain: %w", height, err)
	}
	return pos, entry, nil
}

// AccountFirstIndexedBlock returns the earliest minor block in which this node
// has an indexed record of the account, taken from the first entry of the
// account's main chain index.
//
// ok is false when this node cannot tell — the account has no indexed main
// chain, which is not the same as the account not existing. Callers must not
// turn "I cannot tell" into "it was not there".
func AccountFirstIndexedBlock(account *database.Account) (block uint64, ok bool, err error) {
	mainIndexChain, err := account.MainChain().Index().Get()
	if err != nil {
		return 0, false, errors.UnknownError.WithFormat("load %v main chain index: %w", account.Url(), err)
	}
	if mainIndexChain.Height() == 0 {
		return 0, false, nil
	}

	entry := new(protocol.IndexEntry)
	err = mainIndexChain.EntryAs(0, entry)
	if err != nil {
		return 0, false, errors.UnknownError.WithFormat("load %v main chain index entry %d: %w", account.Url(), 0, err)
	}
	return entry.BlockIndex, true, nil
}

// BPTRootAt returns the BPT root as of the given minor block height, together
// with the block it is actually the root of, which is the last indexed block at
// or before the requested one.
//
// The root comes from the partition ledger's BptChain, which records one entry
// per block that changed state (`internal/core/execute/v2/block/block_end.go:90`,
// gated on V2Baikonur).
//
// The two chains align index-for-index: measured, bpt[j] is the root as of
// root-index[j].BlockIndex. That is not a coincidence and it is not the naive
// reading. BptChain records the PREVIOUS block's state hash, so the entry for
// block B is written while block B+1 is being processed — one block late — and
// root-index[j] is written during block B itself. Being written late by exactly
// one entry is what makes the positions line up rather than differ.
//
// The consequence is that the newest indexed block's root is not on the chain
// yet; it lands when the next state-changing block commits. So BptChain runs
// exactly one entry shorter than the root index chain (measured: 46,105 against
// 46,106 on MainNet, 390,980 against 390,981 on Kermit), and a request for that
// newest block is refused rather than answered with its predecessor's root.
//
// NOTE ON WHAT THIS ROOT IS AND IS NOT. The BptChain is declared an anchor
// chain and has an index chain allocated, but it is never passed through
// addChainAnchor — its entry is written after enumerateModifiedChains has
// already run, so it never appears in the block's chain updates. Measured: its
// index chain is empty on a simulated ledger and on both live networks. **The
// root returned here is therefore recorded by this node but not anchored into
// the root chain, and so not covered by a quorum signature.** A caller that
// needs a quorum-bound root must use one that reached an anchor(X)-bpt chain,
// which is a sparse subset. Do not present this root as if it were signed.
func BPTRootAt(partition config.NetworkUrl, batch *database.Batch, height uint64) (root [32]byte, block uint64, err error) {
	pos, entry, err := ResolveBlockAtOrBefore(partition, batch, height)
	if err != nil {
		return root, 0, errors.UnknownError.Wrap(err)
	}

	bptChain, err := batch.Account(partition.Ledger()).BptChain().Get()
	if err != nil {
		return root, 0, errors.UnknownError.WithFormat("load bpt chain: %w", err)
	}

	i := int64(pos)
	if i >= bptChain.Height() {
		return root, 0, errors.IncompleteChain.WithFormat(
			"the root for block %d is not on the bpt chain yet; it is recorded when the next state-changing block commits", entry.BlockIndex)
	}

	value, err := bptChain.Entry(i)
	if err != nil {
		return root, 0, errors.UnknownError.WithFormat("load bpt chain entry %d: %w", i, err)
	}
	if len(value) != 32 {
		return root, 0, errors.InternalError.WithFormat("bpt chain entry %d is %d bytes, want 32", i, len(value))
	}
	return *(*[32]byte)(value), entry.BlockIndex, nil
}

// ResolveHistoricalAccountState resolves a request for an account's state at a
// minor block height, returning the root index chain entry of the block a
// receipt would be produced against.
//
// It returns one of three distinguishable refusals, so a client can branch
// without parsing prose:
//
//   - [errors.NotFound] — the height is outside what this node has indexed, or
//     the account had no record at that height;
//   - [errors.IncompleteChain] — the height is indexed but the node retains no
//     BPT history for it. The message names the retained range.
//   - [errors.BadRequest] — height is zero, which means the current state and
//     must not reach here.
//
// It never returns the current block for a historical request.
func ResolveHistoricalAccountState(partition config.NetworkUrl, batch *database.Batch, account *database.Account, height uint64) (*protocol.IndexEntry, error) {
	_, entry, err := ResolveBlockAtOrBefore(partition, batch, height)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Did the account exist? Ask about the height the caller asked about, not
	// the resolved one: an account created between the two did not exist at the
	// height in question, and proving it existed later answers a different
	// question.
	first, ok, err := AccountFirstIndexedBlock(account)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if ok && height < first {
		return nil, errors.NotFound.WithFormat(
			"%v did not exist at block %d; this node's earliest record of it is block %d", account.Url(), height, first)
	}

	retained, err := RetainedBlockRange(partition, batch)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if !retained.Contains(entry.BlockIndex) {
		return nil, errors.IncompleteChain.WithFormat(
			"no BPT history retained for block %d; this node's retained range is %v", entry.BlockIndex, retained)
	}

	return entry, nil
}
