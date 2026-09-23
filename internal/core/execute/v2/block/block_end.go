// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"crypto/sha256"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"sort"
	"strconv"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Close ends the block and returns the block state.
//
// A block that fails to close is never committed, and until now nothing
// released it either: the caller holds an execute.Block, which has no
// Discard, so the batch stayed open, pinning its version of the store. On
// run 20260915T042428Z the Directory held one open for ten hours on every
// node after a receipt failed to build, with every commit since staging its
// pre-images behind it (#4279). What the block holds is released here, on
// the way out with the error.
func (block *Block) Close() (execute.BlockState, error) {
	state, err := block.close()
	if err != nil {
		block.discard()
	}
	return state, err
}

// discard releases what the block holds without committing any of it.
func (block *Block) discard() {
	if block.Batch != nil {
		block.Batch.Discard()
	}
	if block.cache != nil {
		block.cache.Discard()
	}
	if block.staging != nil {
		block.staging.Discard()
	}
}

func (block *Block) close() (execute.BlockState, error) {
	if block.fatal != nil {
		// A shard commit failure may have left a partial write in the
		// block batch (#4149) — refuse to hash or commit it.
		return nil, errors.FatalError.Wrap(block.fatal)
	}

	m := block.Executor
	ledgerUrl := m.Describe.NodeUrl(protocol.Ledger)
	ledger := block.Batch.Account(ledgerUrl)

	r := m.BlockTimers.Start(BlockTimerTypeEndBlock)
	defer m.BlockTimers.Stop(r)
	mExecBlocks.WithLabelValues(m.Describe.PartitionId).Inc()

	// THE BLOCK THIS NODE'S OWN EXECUTOR HAS EXECUTED, recorded where nothing
	// but this node's executor writes.
	//
	// The daemon used to take that number from `<partition>/ledger`, which is
	// an ACCOUNT — and one of the accounts the join's pull fetches from a peer
	// and settles into this store. So what the daemon read at start-up was
	// whatever the previous process's pull left behind, not what this node
	// executed, and the daemon started executing at it
	// (#4344). SystemData is not an account and not in the BPT: no pull
	// writes it, and writing it does not move the state root.
	//
	// It is written with the block, in the block's own batch, so it commits
	// exactly when the block does. A crash between them cannot leave the
	// record ahead of the state.
	err := block.Batch.SystemData(m.Describe.PartitionId).ExecutedBlock().Put(block.Index)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("record this node's executed block: %w", err)
	}

	// Write each stream's advances to its ledger, once per stream (#4169 step
	// 7). Before anything else reads or writes those records: production
	// bumps Produced on the same ledger below.
	err = block.flushStreams()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("flush streams: %w", err)
	}
	block.logStreams()

	// Write each anchor's validator signature set, once per anchor (#4224).
	err = block.flushAnchorSignatures()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("flush anchor signatures: %w", err)
	}

	// Is it time for a major block?
	err = block.shouldOpenMajorBlock()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Process events such as expiring transactions. Depends on
	// shouldOpenMajorBlock.
	err = block.processEvents()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("process event backlog: %w", err)
	}

	// Cross-partition recovery is the Conductor's job
	// (internal/core/crosschain). The executor used to launch its own in-block
	// scan here; it fired first (every block, on every validator) and rooted
	// its range proofs at a directory continuation, reproducing #4086 exactly
	// when anchoring lagged — which is when recovery is needed (#4138).

	// List all of the chains that have been modified. shouldPrepareAnchor
	// relies on this list so this must be done first.
	// The block's segment of the root chain begins before its first root
	// append: every receipt this block builds ends in it (executor spec,
	// "Dispatch"), and the ledger reads it back only from what it writes.
	rootHead0, err := block.Batch.Account(m.Describe.Ledger()).RootChain().Inner().Head().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load root chain head: %w", err)
	}
	block.rootSeg = &merkle.Segment{First: rootHead0.Count, Before: rootHead0.Copy(), MarkMask: block.Batch.Account(m.Describe.Ledger()).RootChain().Inner().MarkMask()}

	err = m.enumerateModifiedChains(block)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Settle the block's produced messages — sequence what leaves the
	// partition, queue what stays (#4146). This must run before the anchor
	// decision: whether an anchor is needed depends on whether anything was
	// SEQUENCED, which is only known after the local/remote split.
	err = block.produceBlockMessages()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("produce block messages: %w", err)
	}

	// Should we send an anchor?
	if block.shouldSendAnchor() {
		block.State.Anchor = &BlockAnchorState{}
	}

	// Do nothing if the block is empty
	if block.State.Empty() {
		return &closedBlock{*block, nil}, nil
	}

	// Record the previous block's state hash it on the BPT chain
	if block.Executor.globals().Active.ExecutorVersion.V2BaikonurEnabled() {
		err := ledger.BptChain().Inner().AddEntry(block.State.PreviousStateHash[:], false)
		if err != nil {
			return nil, err
		}
	}

	m.logger.Debug("Committing",
		"module", "block",
		"height", block.Index,
		"delivered", block.State.Delivered,
		"signed", block.State.Signed,
		"updated", len(block.State.ChainUpdates.Entries),
		"produced", block.State.Produced)
	t := time.Now()

	// Record pending transactions
	err = block.recordTransactionExpiration()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Load the main chain of the minor root
	rootChain, err := ledger.RootChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load root chain: %w", err)
	}
	// The root chain's state before this block anchors anything: with the
	// entries this block appends, it is the segment the block's root receipt
	// is built from, in memory (healing spec, "The cache").
	rootSeg := block.rootSeg

	// Process chain updates
	type chainUpdate struct {
		*protocol.BlockEntry
		DidIndex   bool
		IndexIndex uint64
		Txns       [][]byte
	}
	chains := map[string]*chainUpdate{}
	for _, entry := range block.State.ChainUpdates.Entries {
		// Do not create root chain or BPT entries for the ledger
		if ledgerUrl.Equal(entry.Account) {
			continue
		}

		key := strings.ToLower(entry.Account.String()) + ";" + entry.Chain
		u, anchored := chains[key]
		if !anchored {
			u = new(chainUpdate)
			u.BlockEntry = entry
			chains[key] = u
		}

		account := block.Batch.Account(entry.Account)
		chain, err := account.ChainByName(entry.Chain)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("resolve chain %v of %v: %w", entry.Chain, entry.Account, err)
		}
		// The hash at the entry was known when it was appended; a chain
		// appended to outside the block's record (the signature chain, the
		// BPT chain) is read back for it (#4245).
		hash, ok := block.State.ChainUpdates.EntryHash(entry.Account, entry.Chain, entry.Index)
		if !ok {
			chain2, err := chain.Get()
			if err != nil {
				return nil, errors.UnknownError.WithFormat("load chain %v of %v: %w", entry.Chain, entry.Account, err)
			}
			hash, err = chain2.Entry(int64(entry.Index))
			if err != nil {
				return nil, errors.UnknownError.WithFormat("load entry %d of chain %v of %v: %w", entry.Index, entry.Chain, entry.Account, err)
			}
		}
		u.Txns = append(u.Txns, hash)

		// Only anchor each chain once
		if anchored {
			continue
		}

		// Anchor and index the chain
		u.IndexIndex, u.DidIndex, err = addChainAnchor(rootChain, chain, block.Index)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("add anchor to root chain: %w", err)
		}
		if block.rootPosOf == nil {
			block.rootPosOf = map[string]int64{}
		}
		block.rootPosOf[key] = rootChain.Height() - 1
	}

	// Anchor the BPT chain into the root chain (#4272).
	//
	// It cannot go through the loop above: that skips the ledger account
	// outright, because the ledger owns the root chain and a chain cannot be
	// anchored into itself. The bpt chain lives on the ledger, so it needs an
	// explicit anchor here -- the same treatment the synthetic chain gets
	// below, and for the same reason.
	//
	// Without this the chain is written every block and never anchored, so it
	// has no index chain, and a BPT root can be proven no further than the node
	// asserting it. With it, any root the network has ever produced is provable
	// into a root-chain anchor, which is what makes the second call of an
	// account proof always answerable (#4276).
	if block.Executor.globals().Active.ExecutorVersion.V2KourouEnabled() {
		head, err := ledger.BptChain().Inner().Head().Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load bpt chain head: %w", err)
		}
		if head.Count > 0 {
			_, _, err = addChainAnchor(rootChain, ledger.BptChain(), block.Index)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("anchor the bpt chain: %w", err)
			}
		}
	}

	// Anchor each destination's synthetic chain this block appended to into
	// the root chain, and remember where (executor spec, "One chain per
	// pair, one stage per chain").
	if block.State.Produced > 0 {
		err = m.anchorSynthChains(block, rootChain)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// Record the block ledger LAST, so it names everything this block
	// changed. The synthetic chains never reach the loop over modified
	// chains: enumerateModifiedChains rebuilds the entry list from the
	// batch's updated accounts, and it runs BEFORE produceBlockMessages
	// appends anything to a synthetic chain (and in a sub-batch at that), so
	// the synthetic account is not in that list when the loop runs. (The
	// explicit skip in enumerateModifiedChains is therefore dead code, not
	// the thing that prevents double anchoring.) anchorSynthChains is what
	// anchors them and what names their entries, and it runs after
	// everything else that adds to the list, so the record is only complete
	// once it has.
	//
	// One record keyed by block index, and its hash on the block-ledger
	// chain, so the ledger account's hash commits to what this block changed.
	// Written once; the cost is the block's, not the chain's (executor spec,
	// "The block ledger", invariant 9).
	if block.Executor.globals().Active.ExecutorVersion.V2JiuquanEnabled() {
		bl := new(database.BlockLedger)
		bl.Index = block.Index
		bl.Time = block.Time
		bl.Entries = block.State.ChainUpdates.Entries
		err = recordBlockLedger(ledger, bl)
	} else {
		bl := new(protocol.BlockLedger)
		bl.Url = m.Describe.Ledger().JoinPath(strconv.FormatUint(block.Index, 10))
		bl.Index = block.Index
		bl.Time = block.Time
		bl.Entries = block.State.ChainUpdates.Entries
		err = block.Batch.Account(bl.Url).Main().Put(bl)
	}
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store block ledger: %w", err)
	}

	// Complete the cache's block: the root chain is final, so the receipt
	// from the synthetic chain's anchor to the block's root is built from the
	// segment this block appended. A block that produced nothing is held too,
	// so a later lookup distinguishes "nothing to send" from a miss.
	err = block.completeCacheBlock(rootChain, rootSeg)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Index the root chain
	rootIndexIndex, err := addIndexChainEntry(ledger.RootChain().Index(), &protocol.IndexEntry{
		Source:     uint64(rootChain.Height() - 1),
		BlockIndex: block.Index,
		BlockTime:  &block.Time,
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("add root index chain entry: %w", err)
	}

	// Update the transaction-chain index
	for _, c := range chains {
		if !c.DidIndex {
			continue
		}
		for _, hash := range c.Txns {
			e := new(database.TransactionChainEntry)
			e.Account = c.Account
			e.Chain = c.Chain
			e.ChainIndex = c.IndexIndex
			e.AnchorIndex = rootIndexIndex
			err = indexing.TransactionChain(block.Batch, hash).Add(e)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("store transaction chain index: %w", err)
			}
		}
	}

	// Add transaction-chain index entries for synthetic transactions: each
	// names the destination's chain and that chain's index entry for the block.
	if block.cacheBlock != nil {
		for _, e := range block.cacheBlock.Entries {
			st := block.cacheBlock.Stream(e.Stream)
			if st == nil {
				continue
			}
			err = indexing.TransactionChain(block.Batch, e.Hash[:]).Add(&database.TransactionChainEntry{
				Account:     m.Describe.Synthetic(),
				Chain:       st.ChainName,
				ChainIndex:  st.IndexIndex,
				AnchorIndex: rootIndexIndex,
			})
			if err != nil {
				return nil, errors.UnknownError.WithFormat("store transaction chain index: %w", err)
			}
		}
	}

	// Update major index chains if it's a major block
	err = m.recordMajorBlock(block, rootIndexIndex)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Check if an anchor needs to be sent
	err = m.prepareAnchor(block)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Execute post-update actions
	err = block.executePostUpdateActions()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Retain what this block's BPT update is about to overwrite, so a peer can
	// be served an account or a BPT page as of this block once the Directory
	// has anchored it (#4361). A no-op at a depth of zero, and it never
	// changes the BPT root, so it needs no executor-version gate.
	block.Batch.SetBPTHistory(block.Index, m.BPTHistoryDepth)

	// If retention was off for a state-changing block since the last one it
	// ran at, the history before that block is gone and the horizon must say
	// so. The last state-changing block is the ledger's, not this block's
	// index: a partition does not execute a block at every height, so height
	// arithmetic cannot tell idleness from a gap in retention.
	if m.BPTHistoryDepth > 0 && block.Index > 1 {
		_, prev, err := indexing.ResolveBlockAtOrBefore(m.Describe.PartitionUrl(), block.Batch, block.Index-1)
		if err != nil && !errors.Is(err, errors.NotFound) && !errors.Is(err, errors.IncompleteChain) && !errors.Is(err, errors.NotReady) {
			return nil, errors.UnknownError.WithFormat("resolve the previous state-changing block: %w", err)
		}
		var prevBlock uint64
		if prev != nil {
			prevBlock = prev.BlockIndex
		}
		err = block.Batch.NoteBPTBlock(block.Index, m.BPTHistoryDepth, prevBlock)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// Update the BPT
	err = block.Batch.UpdateBPT()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Update active globals, after everything else is done (don't change logic
	// in the middle of a block)
	var valUp []*execute.ValidatorUpdate
	if !m.isGenesis && !m.globals().Active.Equal(&m.globals().Pending) {
		valUp = execute.DiffValidators(&m.globals().Active, &m.globals().Pending, m.Describe.PartitionId)

		// Publish a SNAPSHOT, never a pointer into the executor's own state.
		// Subscribers KEEP what they are handed — the conductor does
		// c.Globals.Store(e.New) — so handing over &globals.Active makes
		// every later in-place update a write to memory they are still
		// reading. That is #4170: Block.Close's `globals.Active = *Pending
		// .Copy()` raced with the conductor's anchoring path, the validation
		// path and AnchorSigner, and a torn read there decides whether a code
		// path is enabled and who may sign.
		err = m.EventBus.Publish(events.WillChangeGlobals{
			New: m.globals().Pending.Copy(),
			Old: m.globals().Active.Copy(),
		})
		if err != nil {
			return nil, errors.UnknownError.WithFormat("publish globals update: %w", err)
		}
		// REPLACE, do not mutate. Anyone already holding the previous
		// snapshot keeps reading it, unchanged and complete, for as long as
		// they hold it — which is what makes readers outside block execution
		// safe without a lock (#4170).
		next := *m.globals()
		next.Active = *next.Pending.Copy()
		m.globalsPtr.Store(&next)
	}

	m.logger.Debug("Committed", "module", "block", "height", block.Index, "duration", time.Since(t))
	return &closedBlock{*block, valUp}, nil
}

func (b *Block) executePostUpdateActions() error {
	version := b.Executor.globals().Pending.ExecutorVersion
	if b.Executor.globals().Active.ExecutorVersion == version {
		return nil
	}

	switch version {
	case protocol.ExecutorVersionV2Jiuquan:
		// Nothing to migrate. Blocks recorded before activation stay readable
		// through LoadBlockLedger's fall-through to the per-block account, and
		// their BPT entries are not touched here: removing them is a
		// reorganization of the whole history, which is a separate problem
		// (executor spec, "The block ledger", activation and history).
	}
	return nil
}

func (block *Block) recordTransactionExpiration() error {
	if len(block.State.PendingTxns) == 0 && len(block.State.PendingSigs) == 0 {
		return nil
	}

	// Get the currentMajor block height
	currentMajor, err := getMajorHeight(block.Executor.Describe, block.Batch)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Set the expiration height
	var max uint64
	if block.Executor.globals().Active.Globals.Limits.PendingMajorBlocks == 0 {
		max = 14 // default to 2 weeks
	} else {
		max = block.Executor.globals().Active.Globals.Limits.PendingMajorBlocks
	}

	// Parse the schedule
	schedule, err := core.Cron.Parse(block.Executor.globals().Active.Globals.MajorBlockSchedule)
	if err != nil && block.Executor.globals().Active.ExecutorVersion.V2BaikonurEnabled() {
		return errors.UnknownError.Wrap(err)
	}

	// Determine which major block each transaction should expire on
	shouldExpireOn := func(txn *protocol.Transaction) uint64 {
		var count uint64
		switch {
		case !block.Executor.globals().Active.ExecutorVersion.V2BaikonurEnabled():
			// Old logic
			count = max

		case txn.Header.Expire == nil || txn.Header.Expire.AtTime == nil:
			// No expiration specified, use the default
			count = max

		default:
			// Always at least the next major block
			count = 1

			// Increment until the expire time is after the expected major block time
			now := block.Time
			for count < max && txn.Header.Expire.AtTime.After(schedule.Next(now)) {
				now = schedule.Next(now)
				count++
			}
		}

		if count <= 0 || count > max {
			count = max
		}

		return currentMajor + count
	}

	// Expire transactions
	pending := map[uint64][]*url.TxID{}
	for _, txn := range block.State.GetPendingTxns() {
		major := shouldExpireOn(txn)
		pending[major] = append(pending[major], txn.ID())
	}

	// Expire signature sets
	for _, s := range block.State.GetPendingSigs() {
		major := shouldExpireOn(s.Transaction)
		for _, auth := range s.GetAuthorities() {
			pending[major] = append(pending[major], auth.WithTxID(s.Transaction.Hash()))
		}
	}

	// Record the IDs
	ledger := block.Batch.Account(block.Executor.Describe.NodeUrl(protocol.Ledger))
	for major, ids := range pending {
		err = ledger.Events().Major().Pending(major).Add(ids...)
		if err != nil {
			return errors.UnknownError.WithFormat("store pending expirations: %w", err)
		}
	}
	return nil
}

func getMajorHeight(desc execute.DescribeShim, batch *database.Batch) (uint64, error) {
	c := batch.Account(desc.AnchorPool()).MajorBlockChain()
	head, err := c.Head().Get()
	if err != nil {
		return 0, errors.UnknownError.WithFormat("load major block chain head: %w", err)
	}
	if head.Count == 0 {
		return 0, nil
	}

	hash, err := c.Entry(head.Count - 1)
	if err != nil {
		return 0, errors.UnknownError.WithFormat("load major block chain latest entry: %w", err)
	}

	entry := new(protocol.IndexEntry)
	err = entry.UnmarshalBinary(hash)
	if err != nil {
		return 0, errors.EncodingError.WithFormat("decode major block chain entry: %w", err)
	}

	return entry.BlockIndex, nil
}

// anchorSynthChains anchors each destination's synthetic chain this block
// appended to into the root chain, records the chain's index entry for the
// block, remembers the root position for the block's proofs, and names each
// entry it appended in the block ledger.
func (m *Executor) anchorSynthChains(block *Block, rootChain *database.Chain) error {
	if block.cacheBlock == nil {
		return nil
	}
	record := block.Batch.Account(m.Describe.Synthetic())
	keys := make([]string, 0, len(block.cacheBlock.Streams))
	for k := range block.cacheBlock.Streams {
		keys = append(keys, k)
	}
	sort.Strings(keys) // one order on every node: the root chain is hashed
	for _, k := range keys {
		st := block.cacheBlock.Streams[k]
		partition, ok := protocol.ParsePartitionUrl(st.Destination)
		if !ok {
			return errors.InternalError.WithFormat("destination %v is not a partition", st.Destination)
		}
		indexIndex, _, err := addChainAnchor(rootChain, record.SyntheticChain(partition), block.Index)
		if err != nil {
			return errors.UnknownError.WithFormat("anchor synthetic chain to %v: %w", st.Destination, err)
		}
		st.IndexIndex = indexIndex
		st.RootPos = rootChain.Height() - 1 // the anchor just appended
	}

	// Name the entries in the block ledger: ONE BlockEntry PER APPENDED CHAIN
	// ENTRY, carrying that entry's own index, as for every other chain. The
	// block ledger's contract is a list of (account, chain, index) triples
	// (executor spec, "The block ledger"), and every consumer reads the chain
	// AT that index — loadBlockEntry, and through it the block query and the
	// block event stream. One entry per chain per block would say index 0 for
	// every block, so block N would report block 1's synthetic transaction as
	// its own, and a reconstruction driven from the block ledger would
	// recover one entry of the block's n.
	//
	// The index is the position buildSynthTxn appended at, carried in the
	// cache's entry. Emitted by sorted stream and ascending index, so the
	// list is the same on every node: the record is hashed onto the
	// block-ledger chain, and the ledger account's hash is in the BPT.
	byStream := make(map[string][]*synthcache.Entry, len(keys))
	for _, e := range block.cacheBlock.Entries {
		k := synthcache.StreamKey(e.Stream)
		byStream[k] = append(byStream[k], e)
	}
	for _, k := range keys {
		st := block.cacheBlock.Streams[k]
		entries := byStream[k]
		sort.Slice(entries, func(i, j int) bool { return entries[i].Index < entries[j].Index })
		for _, e := range entries {
			block.State.ChainUpdates.DidUpdateChain(&protocol.BlockEntry{
				Account: m.Describe.Synthetic(),
				Chain:   st.ChainName,
				Index:   uint64(e.Index),
			})
		}
	}
	return nil
}

func (b *Block) shouldSendAnchor() bool {
	// Did we make a major block?
	if b.State.MakeMajorBlock > 0 || b.State.MajorBlock != nil {
		return true
	}

	// Did we produce synthetic transactions?
	if b.State.Produced > 0 {
		return true
	}

	var didUpdateOther, didAnchorPartition, didAnchorDirectory bool
	anchor := b.Batch.Account(b.Executor.Describe.AnchorPool())
	for _, c := range b.State.ChainUpdates.Entries {
		// The system ledger is always updated
		if c.Account.Equal(b.Executor.Describe.Ledger()) {
			continue
		}

		// Was some other account updated?
		if !c.Account.Equal(b.Executor.Describe.AnchorPool()) {
			didUpdateOther = true
			continue
		}

		// Check if a partition anchor was received
		chain, err := anchor.ChainByName(c.Chain)
		if err != nil {
			b.Executor.logger.Error("Failed to get chain by name", "error", err, "name", c.Chain)
			continue
		}

		partition, ok := chain.Key().Get(3).(string)
		if chain.Key().Get(2) != "AnchorChain" || !ok {
			continue
		}

		if strings.EqualFold(partition, protocol.Directory) {
			didAnchorDirectory = true
		} else {
			didAnchorPartition = true
		}
	}

	// Send an anchor if any account was updated (other than the system and
	// anchor ledgers) or a partition anchor was received
	if didUpdateOther || didAnchorPartition {
		return true
	}

	if !didAnchorDirectory {
		return false
	}

	// The heartbeat. A block whose only content is a received directory anchor
	// still moves the BPT root, and a reader who queries at that moment gets a
	// receipt against a root nothing will ever carry -- so the second call of an
	// account proof cannot be answered until someone happens to transact
	// (#4277). Anchoring these blocks keeps the directory-anchor cascade
	// running, which keeps every root reachable.
	//
	// From Kourou this is unconditional -- it was AnchorEmptyBlocks, a network
	// global, default false, and a proof that only works on a busy network is
	// not a proof anyone can rely on. But it is rate limited: anchoring every
	// such block would run an idle network at full block rate forever, so at
	// most one anchor per anchorHeartbeatSkip+1 blocks.
	if b.Executor.globals().Active.ExecutorVersion.V2KourouEnabled() {
		var anchorLedger *protocol.AnchorLedger
		err := b.Batch.Account(b.Executor.Describe.AnchorPool()).Main().GetAs(&anchorLedger)
		if err != nil {
			b.Executor.logger.Error("Failed to load the anchor ledger", "error", err)
			return true // Anchoring too often beats not being able to prove
		}
		return b.Index > anchorLedger.LastAnchorBlock+anchorHeartbeatSkip
	}
	return b.Executor.globals().Active.Globals.AnchorEmptyBlocks
}

// anchorHeartbeatSkip is how many blocks the heartbeat may skip before
// anchoring anyway: three, so an idle network anchors on at most every fourth
// block instead of every one.
const anchorHeartbeatSkip = 3

func (x *Executor) prepareAnchor(block *Block) error {
	// Determine if an anchor should be sent
	if block.State.Anchor == nil {
		return nil
	}

	// Update the anchor ledger
	anchorLedger, err := database.UpdateAccount(block.Batch, x.Describe.AnchorPool(), func(ledger *protocol.AnchorLedger) error {
		ledger.MinorBlockSequenceNumber++

		// Where the heartbeat counts from (#4277)
		if x.globals().Active.ExecutorVersion.V2KourouEnabled() {
			ledger.LastAnchorBlock = block.Index
		}

		if block.State.MajorBlock == nil {
			return nil
		}

		ledger.MajorBlockIndex = block.State.MajorBlock.Index
		ledger.MajorBlockTime = block.State.MajorBlock.Time

		if x.globals().Active.ExecutorVersion.V2VandenbergEnabled() {
			return nil
		}

		bvns := x.globals().Active.BvnNames()
		if x.globals().Active.ExecutorVersion.V2BaikonurEnabled() {
			// From Baikonur forward, sort this list so changes in the
			// implementation of BvnNames don't break it
			sort.Strings(bvns)

		} else {
			// Use the ordering of routes to sort the BVN list since that preserves
			// the order used prior to 1.3
			routes := map[string]int{}
			for i, r := range x.globals().Active.Routing.Routes {
				id := strings.ToLower(r.Partition)
				if _, ok := routes[id]; ok {
					continue
				}
				routes[id] = i
			}
			sort.Slice(bvns, func(i, j int) bool {
				return routes[strings.ToLower(bvns[i])] < routes[strings.ToLower(bvns[j])]
			})
		}

		ledger.PendingMajorBlockAnchors = make([]*url.URL, len(bvns))
		for i, bvn := range bvns {
			ledger.PendingMajorBlockAnchors[i] = protocol.PartitionUrl(bvn)
		}
		return nil
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Update the system ledger
	_, err = database.UpdateAccount(block.Batch, x.Describe.Ledger(), func(ledger *protocol.SystemLedger) error {
		switch x.Describe.NetworkType {
		case protocol.PartitionTypeDirectory:
			ledger.Anchor, err = x.buildDirectoryAnchor(block, ledger, anchorLedger)
		case protocol.PartitionTypeBlockValidator:
			ledger.Anchor, err = x.buildPartitionAnchor(block, ledger)
		}
		return errors.UnknownError.Wrap(err)
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return errors.UnknownError.Wrap(err)
}

func (x *Executor) buildDirectoryAnchor(block *Block, systemLedger *protocol.SystemLedger, anchorLedger *protocol.AnchorLedger) (*protocol.DirectoryAnchor, error) {
	// Do not populate the root chain index, root chain anchor, or state tree
	// anchor. Those cannot be populated until the block is complete, thus they
	// cannot be populated until the next block starts.
	anchor := new(protocol.DirectoryAnchor)
	anchor.Source = x.Describe.NodeUrl()
	anchor.MinorBlockIndex = block.Index
	anchor.MajorBlockIndex = block.State.MakeMajorBlock

	if !x.globals().Active.BvnExecutorVersion().V2VandenbergEnabled() {
		anchor.Updates = systemLedger.PendingUpdates
	}

	if block.State.MajorBlock != nil && !x.globals().Active.BvnExecutorVersion().V2VandenbergEnabled() {
		anchor.MakeMajorBlock = anchorLedger.MajorBlockIndex
		anchor.MakeMajorBlockTime = anchorLedger.MajorBlockTime
	}

	// Load the root chain
	// Each receipt is built from memory: this block's segment of the
	// partition's anchor chain (chain package, ChainUpdates.Segments), from
	// the received anchor to the chain's new head, joined to this block's
	// segment of the root chain from where that head's anchor landed. Nothing
	// is read back from the chains: a receipt built from storage depends on
	// what the store still answers, and a store's window turned that into
	// rejected anchors for every run from 20260905T032333Z to 051008Z.
	anchorUrl := x.Describe.NodeUrl(protocol.AnchorPool)
	if block.rootSeg == nil {
		return nil, errors.InternalError.With("the block's root chain segment is missing")
	}
	record := block.Batch.Account(anchorUrl)
	for _, received := range block.State.ReceivedAnchors {
		name := record.AnchorChain(received.Partition).Root().Name()
		key := chain.SegmentKey(anchorUrl, name)
		seg := block.State.ChainUpdates.Segments[key]
		if seg == nil {
			return nil, errors.InternalError.WithFormat("no segment for %s anchor chain: the anchor was received without an append", received.Partition)
		}
		// This chain's spans folded out of order, so the segment has a hole
		// and the receipt below would be built over the wrong span (#4279).
		if err := block.State.ChainUpdates.SegmentError(key); err != nil {
			return nil, errors.UnknownError.WithFormat("%s intermediate anchor chain: %w", received.Partition, err)
		}
		rootPos, ok := block.rootPosOf[key]
		if !ok {
			return nil, errors.InternalError.WithFormat("%s anchor chain was not anchored into the root chain this block", received.Partition)
		}
		anchorReceipt, err := seg.Receipt(received.Index, seg.Last())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("build receipt for entry %d (to %d) of %s intermediate anchor chain: %w", received.Index, seg.Last(), received.Partition, err)
		}
		rootReceipt, err := block.rootSeg.Receipt(rootPos, block.rootSeg.Last())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("build receipt for entry %d (to %d) of the root chain: %w", rootPos, block.rootSeg.Last(), err)
		}
		receipt := new(protocol.PartitionAnchorReceipt)
		receipt.Anchor = received.Body.GetPartitionAnchor()
		receipt.RootChainReceipt, err = anchorReceipt.Combine(rootReceipt)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("combine receipt for entry %d of %s intermediate anchor chain: %w", received.Index, received.Partition, err)
		}
		anchor.Receipts = append(anchor.Receipts, receipt)
	}
	return anchor, nil
}

func (b *Block) produceBlockMessages() error {
	// This is likely unnecessarily cautious, but better safe than sorry. This
	// will prevent any variation in order from causing a consensus failure.
	bvns := b.Executor.globals().Active.BvnNames()
	sort.Strings(bvns)

	/* ***** ACME burn (for credits) ***** */

	// If the active version is Vandenberg and ACME has been burnt
	if b.Executor.globals().Active.ExecutorVersion.V2VandenbergEnabled() &&
		b.State.AcmeBurnt.Sign() > 0 {
		body := new(protocol.SyntheticBurnTokens)
		body.Amount = b.State.AcmeBurnt
		txn := new(protocol.Transaction)
		txn.Header.Principal = protocol.AcmeUrl()
		txn.Body = body
		msg := new(messaging.TransactionMessage)
		msg.Transaction = txn

		b.produced = append(b.produced, &ProducedMessage{
			Destination: protocol.AcmeUrl(),
			Message:     msg,
		})
	}

	/* ***** Network account updates (DN) ***** */

	// If the active version is Vandenberg, we're on the DN, and there's a
	// network update
	if b.Executor.globals().Active.ExecutorVersion.V2VandenbergEnabled() &&
		b.Executor.Describe.NetworkType == protocol.PartitionTypeDirectory &&
		len(b.State.NetworkUpdate) > 0 {
		for _, bvn := range bvns {
			b.produced = append(b.produced, &ProducedMessage{
				Destination: protocol.PartitionUrl(bvn),
				Message: &messaging.NetworkUpdate{
					Accounts: b.State.NetworkUpdate,
				},
			})
		}
	}

	/* ***** Major block notification ***** */

	// If the active version is Vandenberg, we're on the DN, and there's a major
	// block
	if b.Executor.globals().Active.ExecutorVersion.V2VandenbergEnabled() &&
		b.Executor.Describe.NetworkType == protocol.PartitionTypeDirectory &&
		b.State.MajorBlock != nil {
		for _, bvn := range bvns {
			b.produced = append(b.produced, &ProducedMessage{
				Destination: protocol.PartitionUrl(bvn),
				Message: &messaging.MakeMajorBlock{
					MajorBlockIndex: b.State.MajorBlock.Index,
					MajorBlockTime:  b.State.MajorBlock.Time,
					MinorBlockIndex: b.Index,
				},
			})
		}
	}

	/* ***** Did update version (BVN) ***** */

	// If the **pending** version is Vandenberg, we're on a BVN, and the version
	// is changing
	if b.Executor.globals().Pending.ExecutorVersion.V2VandenbergEnabled() &&
		b.Executor.Describe.NetworkType != protocol.PartitionTypeDirectory &&
		b.Executor.globals().Pending.ExecutorVersion != b.Executor.globals().Active.ExecutorVersion {
		b.produced = append(b.produced, &ProducedMessage{
			Destination: protocol.DnUrl(),
			Message: &messaging.DidUpdateExecutorVersion{
				Partition: b.Executor.Describe.PartitionId,
				Version:   b.Executor.globals().Pending.ExecutorVersion,
			},
		})
	}

	// Settle the block's produced messages — the deliveries' and the system's
	// alike — in ONE sorted pass (#4144). The sort key is derived from the
	// produced set alone — (producer transaction ID, emission index), with
	// producer-less system messages after — so nothing depends on delivery
	// order, which under parallel execution (#4145) is a shard-scheduling
	// accident.
	//
	// Locally routed messages go on the next-block queue instead of being
	// sequenced (#4146): no sequence number, no synthetic main chain
	// position — so they consume no collection-proof span — no dispatch, and
	// no anchoring dependency. Only what actually leaves the partition is
	// counted as produced, so the synthetic chain is anchored only when it
	// grew.
	sortProduced(b.produced)
	remote, err := b.splitLocalDeliveries(b.produced)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	b.cacheBlock, err = b.Executor.produceSyntheticInto(b.Batch, remote, b.Index, b.cache)
	if err != nil {
		return errors.UnknownError.WithFormat("sequence produced messages: %w", err)
	}
	b.State.Produced += len(remote)
	b.logProduced()
	b.produced = nil

	return nil
}

func (x *Executor) buildPartitionAnchor(block *Block, ledger *protocol.SystemLedger) (*protocol.BlockValidatorAnchor, error) {
	// Do not populate the root chain index, root chain anchor, or state tree
	// anchor. Those cannot be populated until the block is complete, thus they
	// cannot be populated until the next block starts.
	anchor := new(protocol.BlockValidatorAnchor)
	anchor.Source = x.Describe.NodeUrl()
	anchor.MinorBlockIndex = block.Index
	anchor.MajorBlockIndex = block.State.MakeMajorBlock

	if !x.globals().Active.ExecutorVersion.V2VandenbergEnabled() {
		anchor.AcmeBurnt = ledger.AcmeBurnt
	}

	return anchor, nil
}

func (x *Executor) enumerateModifiedChains(block *Block) error {
	block.State.ChainUpdates.Entries = nil

	// For each modified account
	for _, account := range block.Batch.UpdatedAccounts() {
		chains, err := account.UpdatedChains()
		if err != nil {
			return errors.UnknownError.WithFormat("get updated chains of %v: %w", account.Url(), err)
		}

		// For each modified chain
		for _, e := range chains {
			// Anchoring the synthetic transaction ledger causes sadness and
			// despair (it breaks things but I don't know why)
			//
			// This branch never fires. The only place the partition's
			// synthetic chain is appended is buildSynthTxn, reached from
			// produceBlockMessages, which runs after this enumeration and
			// writes into a sub-batch committed later still -- so the
			// synthetic account is never among UpdatedAccounts here.
			// Instrumented over a two-BVN simulator run: 0 hits against
			// 6,951 chains enumerated. It is left in place because removing
			// it is a separate question; it is not what keeps a synthetic
			// chain from being anchored twice (see "Record the block ledger
			// LAST").
			_, ok := protocol.ParsePartitionUrl(e.Account)
			if ok && e.Account.PathEqual(protocol.Synthetic) {
				continue
			}

			// Add a block entry
			block.State.ChainUpdates.Entries = append(block.State.ChainUpdates.Entries, e)
		}
	}

	// [database.Batch.UpdatedAccounts] iterates over a map and thus returns
	// the accounts in a random order. Since randomness and distributed
	// consensus do not mix, the entries are sorted to ensure a consistent
	// ordering.
	//
	// Modified chains are anchored into the root chain later in this
	// function. This combined with the fact that the entries are sorted
	// here means that the anchors in the root chain and entries in the
	// block ledger will be recorded in a well-defined sort order.
	//
	// Another side effect of sorting is that information about the ordering
	// of transactions is lost. That could be viewed as a problem; however,
	// we intend on parallelizing transaction processing in the future.
	// Declaring that the protocol does not preserve transaction ordering
	// will make it easier to parallelize transaction processing.
	e := block.State.ChainUpdates.Entries
	sort.Slice(e, func(i, j int) bool { return e[i].Compare(e[j]) < 0 })

	return nil
}

// recordBlockLedger writes a block's ledger record and appends its hash to the
// block-ledger chain.
func recordBlockLedger(ledger *database.Account, bl *database.BlockLedger) error {
	err := ledger.BlockLedger(bl.Index).Put(bl)
	if err != nil {
		return errors.UnknownError.WithFormat("store block ledger: %w", err)
	}
	data, err := bl.MarshalBinary()
	if err != nil {
		return errors.UnknownError.WithFormat("marshal block ledger: %w", err)
	}
	h := sha256.Sum256(data)
	err = ledger.BlockLedgerChain().Inner().AddEntry(h[:], false)
	if err != nil {
		return errors.UnknownError.WithFormat("add block ledger chain entry: %w", err)
	}
	return nil
}

// completeCacheBlock finishes what the producer's cache holds for this block
// (healing spec, "The cache"): the root receipt from the synthetic chain's
// anchor to the block's root, built from the root chain segment this block
// appended, in memory; the Directory anchors the block executed, for the
// next block's dispatch; and the block record itself, present even when
// nothing was produced so a lookup can tell "nothing to send" from a miss.
func (block *Block) completeCacheBlock(rootChain *database.Chain, rootSeg *merkle.Segment) error {
	blk := block.cacheBlock
	if blk == nil {
		blk = &synthcache.Block{Index: block.Index, Streams: map[string]*synthcache.Stream{}}
		block.cacheBlock = blk
	}
	// Extend the block's root segment with what this block appended: its own
	// writes, read back from the batch, never from history.
	height := rootChain.Height()
	for i := rootSeg.First + int64(len(rootSeg.Elements)); i < height; i++ {
		h, err := rootChain.Entry(i)
		if err != nil {
			return errors.UnknownError.WithFormat("load root chain entry %d: %w", i, err)
		}
		rootSeg.Append(h)
	}
	// Each destination's chain was anchored into the root chain at its own
	// position; the receipt from there to the block's root completes what a
	// proof for that destination's entries is built from.
	for _, st := range blk.Streams {
		receipt, err := rootSeg.Receipt(st.RootPos, height-1)
		if err != nil {
			return errors.UnknownError.WithFormat("build root receipt %d..%d for %v: %w", st.RootPos, height-1, st.Destination, err)
		}
		st.RootReceipt = receipt
	}
	for _, r := range block.State.ReceivedAnchors {
		if da, ok := r.Body.(*protocol.DirectoryAnchor); ok {
			block.cache.AddReceived(da)
		}
	}
	// What each source said it has executed of ours: released from the cache
	// when the block commits (healing spec, "The cache")
	for _, a := range block.remoteDelivered {
		block.cache.Release(a.source, a.delivered)
	}
	block.cache.SetBlock(blk)
	return nil
}
