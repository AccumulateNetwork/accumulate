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
// And the refusal has to say which kind it is. On this line a requester treats
// NotFound as a fact about the RECORD and a capability limit as a fact about
// the PEER (`join/sources.go`), so a node that answers NotFound because it
// could not read its own index makes the requester drop an account the network
// holds. Nothing in this file turns a store miss into NotFound.
//
// Retention is per-node configuration (BPTHistoryDepth). A node running a depth
// of zero retains nothing, refuses every historical request with
// [errors.IncompleteChain], and is a correct, honest node — but it cannot serve
// a join on this line, which is why the depth defaults to non-zero here
// (#4361).

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
// The range is PREDICTIVE: every height inside it is answerable and every height
// below it is refused. That is the whole value of advertising it, so both ends
// are computed from what will actually be served rather than from what was
// retained, which is not the same thing.
//
//   - The earliest end is read from what the node ACTUALLY retained, never from
//     its configured depth — raising the depth does not retroactively create
//     history — and is then rounded UP to the first indexed block at or after
//     it. A height resolves backward to the last state-changing block at or
//     before it, so a height sitting between the retention horizon and the next
//     state-changing block resolves below the horizon and is refused. Advertising
//     it would over-promise by up to one inter-block gap.
//   - The latest end is the newest block whose root is on the ledger's BptChain,
//     which is one entry SHORTER than the root index chain: BptChain records the
//     previous block's state hash, so the newest indexed block's root lands only
//     when the next state-changing block commits. Advertising the newest indexed
//     block would over-promise by exactly one.
//
// An empty range means the node retains no history, which is every node running
// the default configuration.
func RetainedBlockRange(partition config.NetworkUrl, batch *database.Batch) (BlockRange, error) {
	empty := BlockRange{Earliest: 1, Latest: 0}

	horizon, ok, err := batch.BPT().EarliestRetained()
	if err != nil {
		return BlockRange{}, errors.UnknownError.WithFormat("load earliest retained height: %w", err)
	}
	if !ok {
		return empty, nil
	}

	rootIndexChain, err := batch.Account(partition.Ledger()).RootChain().Index().Get()
	if err != nil {
		return BlockRange{}, errors.UnknownError.WithFormat("load minor root index chain: %w", err)
	}
	if rootIndexChain.Height() == 0 {
		return empty, nil
	}

	bptChain, err := batch.Account(partition.Ledger()).BptChain().Get()
	if err != nil {
		return BlockRange{}, errors.UnknownError.WithFormat("load bpt chain: %w", err)
	}
	if bptChain.Height() == 0 {
		return empty, nil
	}

	// The newest block whose root is recorded
	last := new(protocol.IndexEntry)
	err = rootIndexChain.EntryAs(bptChain.Height()-1, last)
	if err != nil {
		return BlockRange{}, errors.UnknownError.WithFormat("load minor root index chain entry %d: %w", bptChain.Height()-1, err)
	}

	// The first indexed block at or after the retention horizon
	_, first, err := SearchIndexChain(rootIndexChain, uint64(rootIndexChain.Height())-1, MatchAfter, SearchIndexChainByBlock(horizon))
	if err != nil {
		// Every indexed block predates the horizon, so nothing is answerable
		return empty, nil
	}

	if first.BlockIndex > last.BlockIndex {
		return empty, nil
	}
	return BlockRange{Earliest: first.BlockIndex, Latest: last.BlockIndex}, nil
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
		return 0, nil, errors.IncompleteChain.WithFormat(
			"block %d precedes this node's earliest indexed block %d", height, indexed.Earliest)
	}

	// Beyond what this node has indexed. The state at H may well equal the
	// state at the latest indexed block — but this node cannot tell a recent
	// empty block from a block that has not happened, and guessing which would
	// mean answering for a block that may not exist. This is "not yet", not
	// "never".
	if height > indexed.Latest {
		return 0, nil, errors.NotReady.WithFormat(
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
// chain, or it has one whose beginning this node does not hold. Neither is the
// same as the account not existing, and callers must not turn "I cannot tell"
// into "it was not there".
//
// # A JOINED NODE CANNOT TELL, AND MUST NOT SAY NOT-FOUND
//
// A node that pulled its state holds each non-spine chain with its OPEN MARK
// SET only (`pull.go`, ModeStateOnly: `lastMark := want.Count &^ MarkMask()`),
// so for a main index chain longer than one mark block — 256 entries, since
// markPower is 8 — element 0 is not in the store. Measured on this line with
// the production pull:
//
//	PULLED AccountFirstIndexedBlock => ok=false
//	  err=load acc://alice/tokens main chain index entry 0: cannot locate element 0
//	  code=notFound
//
// Letting that out as an error would be worse than useless. [errors.Code] walks
// a wrapping UnknownError down to the cause (`pkg/errors/inspect.go`), so the
// client sees NotFound; `join/sources.go` makes a NotFound that EVERY peer
// gives the network's answer about the record; and on a network whose reachable
// peers have all joined, that is every peer. The requester would conclude the
// account did not exist at that height and drop an account all of them hold.
//
// So a store miss on element 0 is "I cannot tell", not an error and never a
// status. The caller then skips the existence question and answers from the
// BPT, which is the authority for it anyway.
// TestAJoinedNodeDoesNotCallItsOwnGapAnAbsence stands the joined node.
func AccountFirstIndexedBlock(account *database.Account) (block uint64, ok bool, err error) {
	mainIndexChain, err := account.MainChain().Index().Get()
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return 0, false, nil
	default:
		return 0, false, errors.UnknownError.WithFormat("load %v main chain index: %w", account.Url(), err)
	}
	if mainIndexChain.Height() == 0 {
		return 0, false, nil
	}

	entry := new(protocol.IndexEntry)
	err = mainIndexChain.EntryAs(0, entry)
	switch {
	case err == nil:
		return entry.BlockIndex, true, nil
	case errors.Is(err, errors.NotFound):
		return 0, false, nil // The beginning of the chain is not held here
	default:
		return 0, false, errors.UnknownError.WithFormat("load %v main chain index entry %d: %w", account.Url(), 0, err)
	}
}

// BPTRootAt returns the BPT root as of the given minor block height, together
// with the block it is actually the root of, which is the last indexed block at
// or before the requested one.
//
// The root comes from the partition ledger's BptChain, which records one entry
// per block that changed state (`internal/core/execute/v2/block/block_end.go:90`,
// gated on V2Baikonur).
//
// The two chains align index-for-index: bpt[j] is the root as of
// root-index[j].BlockIndex. That is not the naive reading. BptChain records the
// PREVIOUS block's state hash, so the entry for block B is written while the
// next state-changing block is being processed — one entry late — and
// root-index[j] is written during block B itself. Being written late by exactly
// one entry is what makes the positions line up rather than differ.
//
// The consequence is that the newest indexed block's root is not on the chain
// yet; it lands when the next state-changing block commits. So BptChain runs
// exactly one entry shorter than the root index chain, and a request for that
// newest block is refused rather than answered with its predecessor's root.
//
// THIS IS RE-DERIVED ON THIS LINE, NOT INHERITED. The alignment is a property
// of this line's block_end and its genesis, both of which differ from `main`'s,
// and ElementIndex-style positional assumptions are what #4321/#4327/#4328/
// #4330 are about. TestTheBptChainAndTheRootIndexChainAlignOnThisLine asserts
// it block by block against the StateTreeAnchor the Directory holds. Anything
// it misses is caught rather than served: GetReceiptAt refuses when the tree
// reconstructed from retained nodes does not hash to the root passed here.
//
// WHAT THIS ROOT IS ON THIS LINE. It is the value the partition's anchor for
// that block carries as its StateTreeAnchor: the anchor is built in the
// following block's BeginBlock from the BPT root as it stood at the end of this
// one (internal/core/crosschain/anchoring.go). So a node holding a verified
// anchor for block B holds exactly this value for B, and a receipt terminating
// here is one it can check. It is also bindable: unlike `main`'s note, this
// line anchors the ledger's bpt chain into the root chain under Kourou
// (internal/core/execute/v2/block/block_end.go, #4272), so any root on that
// chain extends to a root-chain anchor and binds to a directory root through
// ProofService.AnchorReceipt (#4276).
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
// It returns one of four distinguishable refusals, so a client can branch
// without parsing prose:
//
//   - [errors.NotFound] — this node's index has no record of the account at
//     that height. It is not proof of absence: it is what THIS node's index
//     says, so a requester counts it as a miss and asks somebody else. It is
//     never returned for anything this node merely could not read.
//   - [errors.IncompleteChain] — a capability limit: the height precedes what
//     this node has indexed, or is indexed but the node retains no BPT history
//     for it. The message names the boundary.
//   - [errors.NotReady] — the height is beyond this node's latest indexed
//     block: "not yet", not "never". The client should retry later.
//   - [errors.BadRequest] — height is zero, which means the current state and
//     must not reach here.
//
// It never returns the current block for a historical request.
func ResolveHistoricalAccountState(partition config.NetworkUrl, batch *database.Batch, account *database.Account, height uint64) (*protocol.IndexEntry, error) {
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
			"this node's earliest record of %v is block %d, so it has none at block %d", account.Url(), first, height)
	}

	return ResolveRetainedBlock(partition, batch, height)
}

// ResolveRetainedBlock resolves a requested minor block height to the last
// state-changing block at or before it that this node can produce historical
// BPT state for, and refuses distinguishably when it cannot.
//
// It is what [ResolveHistoricalAccountState] does minus the question of whether
// one account existed, so a whole-tree request — a BPT page as of a block
// (#4361) — refuses on the same boundaries and with the same status codes as an
// account request does.
func ResolveRetainedBlock(partition config.NetworkUrl, batch *database.Batch, height uint64) (*protocol.IndexEntry, error) {
	_, entry, err := ResolveBlockAtOrBefore(partition, batch, height)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
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
