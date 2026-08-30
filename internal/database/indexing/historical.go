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

// RetainedBlockRange returns the range of minor blocks for which this node
// retains enough superseded BPT state to produce a historical membership
// receipt.
//
// depth is how many blocks of history the node is configured to keep. Zero —
// the default, and the only value any node uses today — yields an empty range:
// the node retains no history and refuses every historical request.
func RetainedBlockRange(partition config.NetworkUrl, batch *database.Batch, depth uint64) (BlockRange, error) {
	if depth == 0 {
		return BlockRange{Earliest: 1, Latest: 0}, nil // Empty
	}

	indexed, err := IndexedBlockRange(partition, batch)
	if err != nil {
		return BlockRange{}, errors.UnknownError.Wrap(err)
	}

	// The retained window is the last depth blocks, clipped to what is indexed
	earliest := indexed.Earliest
	if indexed.Latest >= depth && indexed.Latest-depth+1 > earliest {
		earliest = indexed.Latest - depth + 1
	}
	return BlockRange{Earliest: earliest, Latest: indexed.Latest}, nil
}

// ResolveBlockAtOrAfter resolves a requested minor block height to the first
// block at or after it that the partition ledger has indexed, and returns that
// root index chain entry.
//
// Resolution moves forward, never backward: the caller asked about height H and
// gets a block no earlier than H, so the answer cannot silently predate the
// question. The resolved height may exceed the requested one, which is why
// callers must report the resolved height rather than the requested one.
func ResolveBlockAtOrAfter(partition config.NetworkUrl, batch *database.Batch, height uint64) (*protocol.IndexEntry, error) {
	if height == 0 {
		return nil, errors.BadRequest.With("cannot resolve block zero: zero means the current state")
	}

	indexed, err := IndexedBlockRange(partition, batch)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Below this node's horizon. Do not resolve forward — see IndexedBlockRange.
	if height < indexed.Earliest {
		return nil, errors.NotFound.WithFormat(
			"block %d precedes this node's earliest indexed block %d", height, indexed.Earliest)
	}

	// Not reached yet. This is "not yet", not "never".
	if height > indexed.Latest {
		return nil, errors.NotFound.WithFormat(
			"block %d has not been reached; this node's latest indexed block is %d", height, indexed.Latest)
	}

	rootIndexChain, err := batch.Account(partition.Ledger()).RootChain().Index().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load minor root index chain: %w", err)
	}

	_, entry, err := SearchIndexChain(rootIndexChain, uint64(rootIndexChain.Height())-1, MatchAfter, SearchIndexChainByBlock(height))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("locate index entry for block %d of the minor root chain: %w", height, err)
	}
	return entry, nil
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
func ResolveHistoricalAccountState(partition config.NetworkUrl, batch *database.Batch, account *database.Account, height, retainedDepth uint64) (*protocol.IndexEntry, error) {
	entry, err := ResolveBlockAtOrAfter(partition, batch, height)
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

	retained, err := RetainedBlockRange(partition, batch, retainedDepth)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if !retained.Contains(entry.BlockIndex) {
		return nil, errors.IncompleteChain.WithFormat(
			"no BPT history retained for block %d; this node's retained range is %v", entry.BlockIndex, retained)
	}

	return entry, nil
}
