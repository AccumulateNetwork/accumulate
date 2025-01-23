// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Close ends the block and returns the block state.
func (block *Block) Close() (execute.BlockState, error) {
	m := block.Executor
	ledgerUrl := m.Describe.NodeUrl(protocol.Ledger)
	ledger := block.Batch.Account(ledgerUrl)

	r := m.BlockTimers.Start(BlockTimerTypeEndBlock)
	defer m.BlockTimers.Stop(r)

	// Is it time for a major block?
	err := block.shouldOpenMajorBlock()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Process events such as expiring transactions. Depends on
	// shouldOpenMajorBlock.
	err = block.processEvents()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("process event backlog: %w", err)
	}

	// Check for missing synthetic transactions. Load the ledger synchronously,
	// request transactions asynchronously.
	var synthLedger *protocol.SyntheticLedger
	err = block.Batch.Account(m.Describe.Synthetic()).Main().GetAs(&synthLedger)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic ledger: %w", err)
	}
	var anchorLedger *protocol.AnchorLedger
	err = block.Batch.Account(m.Describe.AnchorPool()).Main().GetAs(&anchorLedger)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic ledger: %w", err)
	}

	if m.EnableHealing {
		m.BackgroundTaskLauncher(func() { m.requestMissingSyntheticTransactions(block.Index, synthLedger, anchorLedger) })
	}

	// List all of the chains that have been modified. shouldPrepareAnchor
	// relies on this list so this must be done first.
	err = m.enumerateModifiedChains(block)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Should we send an anchor?
	if block.shouldSendAnchor() {
		block.State.Anchor = &BlockAnchorState{}
	}

	// Send messages produced by the block. Depends on shouldPrepareAnchor.
	err = block.produceBlockMessages()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("produce block messages: %w", err)
	}

	// Do nothing if the block is empty
	if block.State.Empty() {
		return &closedBlock{*block, nil}, nil
	}

	// Record the previous block's state hash it on the BPT chain
	if block.Executor.globals.Active.ExecutorVersion.V2BaikonurEnabled() {
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
		chain2, err := chain.Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load chain %v of %v: %w", entry.Chain, entry.Account, err)
		}
		hash, err := chain2.Entry(int64(entry.Index))
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load entry %d of chain %v of %v: %w", entry.Index, entry.Chain, entry.Account, err)
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
	}

	// Record the block entries
	if block.Executor.globals.Active.ExecutorVersion.V2JiuquanEnabled() {
		bl := new(database.BlockLedger)
		bl.Index = block.Index
		bl.Time = block.Time
		bl.Entries = block.State.ChainUpdates.Entries
		err = ledger.BlockLedger().Append(record.NewKey(block.Index), bl)
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

	// Add the synthetic transaction chain to the root chain
	var synthIndexIndex uint64
	if block.State.Produced > 0 {
		synthIndexIndex, err = m.anchorSynthChain(block, rootChain)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
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

	// Add transaction-chain index entries for synthetic transactions
	for _, e := range block.State.ChainUpdates.SynthEntries {
		err = indexing.TransactionChain(block.Batch, e.Transaction).Add(&database.TransactionChainEntry{
			Account:     m.Describe.Synthetic(),
			Chain:       protocol.MainChain,
			ChainIndex:  synthIndexIndex,
			AnchorIndex: rootIndexIndex,
		})
		if err != nil {
			return nil, errors.UnknownError.WithFormat("store transaction chain index: %w", err)
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

	// Update the BPT
	err = block.Batch.UpdateBPT()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Update active globals, after everything else is done (don't change logic
	// in the middle of a block)
	var valUp []*execute.ValidatorUpdate
	if !m.isGenesis && !m.globals.Active.Equal(&m.globals.Pending) {
		valUp = execute.DiffValidators(&m.globals.Active, &m.globals.Pending, m.Describe.PartitionId)

		err = m.EventBus.Publish(events.WillChangeGlobals{
			New: &m.globals.Pending,
			Old: &m.globals.Active,
		})
		if err != nil {
			return nil, errors.UnknownError.WithFormat("publish globals update: %w", err)
		}
		m.globals.Active = *m.globals.Pending.Copy()
	}

	m.logger.Debug("Committed", "module", "block", "height", block.Index, "duration", time.Since(t))
	return &closedBlock{*block, valUp}, nil
}

func (b *Block) executePostUpdateActions() error {
	version := b.Executor.globals.Pending.ExecutorVersion
	if b.Executor.globals.Active.ExecutorVersion == version {
		return nil
	}

	switch version {
	case protocol.ExecutorVersionV2Jiuquan:
		// For each previously recorded block, add a placeholder to the new
		// block ledger and delete the BPT entry for the old block ledger
		// account
		acct := b.Batch.Account(b.Executor.Describe.Ledger())
		chain := acct.RootChain().Index()
		head, err := chain.Head().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load root index chain head: %w", err)
		}

		var entry protocol.IndexEntry
		for i := 0; i < int(head.Count); i += 256 {
			entries, err := chain.Inner().Entries(int64(i), int64(i+256))
			if err != nil {
				return errors.UnknownError.WithFormat("load root index chain entries (%d, %d): %w", i, i+255, err)
			}

			for j, data := range entries {
				entry = protocol.IndexEntry{}
				err = entry.UnmarshalBinary(data)
				if err != nil {
					return errors.UnknownError.WithFormat("decode root index chain entry %d: %w", i+j, err)
				}
				err = acct.BlockLedger().Append(record.NewKey(entry.BlockIndex), &database.BlockLedger{})
				if err != nil {
					return errors.UnknownError.WithFormat("add ledger entry for block %d: %w", i+j, err)
				}
				blockLedgerAccount := b.Batch.Account(b.Executor.Describe.BlockLedger(entry.BlockIndex))
				err = b.Batch.BPT().Delete(blockLedgerAccount.Key())
				if err != nil {
					return errors.UnknownError.WithFormat("delete BPT entry for block ledger %d: %w", i+j, err)
				}
			}
		}
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
	if block.Executor.globals.Active.Globals.Limits.PendingMajorBlocks == 0 {
		max = 14 // default to 2 weeks
	} else {
		max = block.Executor.globals.Active.Globals.Limits.PendingMajorBlocks
	}

	// Parse the schedule
	schedule, err := core.Cron.Parse(block.Executor.globals.Active.Globals.MajorBlockSchedule)
	if err != nil && block.Executor.globals.Active.ExecutorVersion.V2BaikonurEnabled() {
		return errors.UnknownError.Wrap(err)
	}

	// Determine which major block each transaction should expire on
	shouldExpireOn := func(txn *protocol.Transaction) uint64 {
		var count uint64
		switch {
		case !block.Executor.globals.Active.ExecutorVersion.V2BaikonurEnabled():
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

// anchorSynthChain anchors the synthetic transaction chain.
func (m *Executor) anchorSynthChain(block *Block, rootChain *database.Chain) (indexIndex uint64, err error) {
	url := m.Describe.Synthetic()
	indexIndex, _, err = addChainAnchor(rootChain, block.Batch.Account(url).MainChain(), block.Index)
	if err != nil {
		return 0, errors.UnknownError.Wrap(err)
	}

	err = block.Batch.SystemData(m.Describe.PartitionId).SyntheticIndexIndex(block.Index).Put(indexIndex)
	if err != nil {
		return 0, errors.UnknownError.WithFormat("store synthetic transaction index index for block: %w", err)
	}

	block.State.ChainUpdates.DidUpdateChain(&protocol.BlockEntry{
		Account: url,
		Chain:   protocol.MainChain,
	})

	return indexIndex, nil
}

func (x *Executor) requestMissingSyntheticTransactions(blockIndex uint64, synthLedger *protocol.SyntheticLedger, anchorLedger *protocol.AnchorLedger) {
	batch := x.Database.Begin(false)
	defer batch.Discard()

	// Setup
	dispatcher := x.NewDispatcher()
	defer dispatcher.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// For each partition
	for _, partition := range synthLedger.Sequence {
		x.requestMissingTransactionsFromPartition(ctx, dispatcher, partition, false)
	}
	for _, partition := range anchorLedger.Sequence {
		x.requestMissingTransactionsFromPartition(ctx, dispatcher, partition, true)
	}

	for err := range dispatcher.Send(ctx) {
		switch err := err.(type) {
		case protocol.TransactionStatusError:
			x.logger.Error("Failed to dispatch transactions", "block", blockIndex, "error", err, "stack", err.TransactionStatus.Error.PrintFullCallstack(), "txid", err.TxID)
		default:
			x.logger.Error("Failed to dispatch transactions", "block", blockIndex, "error", err, "stack", fmt.Sprintf("%+v\n", err))
		}
	}
}

func (x *Executor) requestMissingTransactionsFromPartition(ctx context.Context, dispatcher Dispatcher, partition *protocol.PartitionSyntheticLedger, anchor bool) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// For each pending synthetic transaction
	dest := x.Describe.NodeUrl()
	for i, txid := range partition.Pending {
		// If we know the ID we must have a local copy (so we don't need to
		// fetch it)
		if txid != nil {
			continue
		}

		seqNum := partition.Delivered + uint64(i) + 1
		message := "Missing synthetic transaction"
		src := partition.Url.JoinPath(protocol.Synthetic)
		if anchor {
			message = "Missing anchor transaction"
			src = partition.Url.JoinPath(protocol.AnchorPool)
		}
		x.logger.Info(message, "seq-num", seqNum, "source", partition.Url)

		// Request the transaction by sequence number
		resp, err := x.Sequencer.Sequence(ctx, src, dest, seqNum, private.SequenceOptions{})
		if err != nil {
			x.logger.Error("Failed to request sequenced transaction", "error", err, "from", src, "seq-num", seqNum)
			continue
		}

		// Sanity check: the response includes a transaction
		if resp.Message == nil {
			x.logger.Error("Response to query-synth is missing the transaction", "from", partition.Url, "seq-num", seqNum, "is-anchor", anchor)
			continue
		}
		if resp.Sequence == nil || resp.Sequence.Source == nil {
			x.logger.Error("Response to query-synth is missing the source", "from", partition.Url, "seq-num", seqNum, "is-anchor", anchor)
			continue
		}
		if resp.Signatures == nil {
			x.logger.Error("Response to query-synth is missing the signatures", "from", partition.Url, "seq-num", seqNum, "is-anchor", anchor)
			continue
		}
		if !anchor {
			if resp.SourceReceipt == nil {
				x.logger.Error("Response to query-synth is missing the proof", "from", partition.Url, "seq-num", seqNum, "is-anchor", anchor)
				continue
			}
		}

		seq := &messaging.SequencedMessage{
			Message:     resp.Message,
			Source:      resp.Sequence.Source,
			Destination: resp.Sequence.Destination,
			Number:      resp.Sequence.Number,
		}

		keySig, bad := x.getKeySignature(resp, partition, seq, anchor)
		if keySig == nil {
			x.logger.Error("Invalid anchor transaction", "error", "missing key signature", "hash", logging.AsHex(resp.Message.Hash()).Slice(0, 4))
			bad = true
		}
		if bad {
			continue
		}

		var msg messaging.Message
		if anchor {
			msg = &messaging.BlockAnchor{
				Anchor:    seq,
				Signature: keySig,
			}
		} else if x.globals.Active.ExecutorVersion.V2BaikonurEnabled() {
			msg = &messaging.SyntheticMessage{
				Message:   seq,
				Signature: keySig,
				Proof: &protocol.AnnotatedReceipt{
					Receipt: resp.SourceReceipt,
					Anchor: &protocol.AnchorMetadata{
						Account: protocol.DnUrl(),
					},
				},
			}
		} else {
			msg = &messaging.BadSyntheticMessage{
				Message:   seq,
				Signature: keySig,
				Proof: &protocol.AnnotatedReceipt{
					Receipt: resp.SourceReceipt,
					Anchor: &protocol.AnchorMetadata{
						Account: protocol.DnUrl(),
					},
				},
			}
		}

		err = dispatcher.Submit(ctx, dest, &messaging.Envelope{Messages: []messaging.Message{msg}})
		if err != nil {
			x.logger.Error("Failed to dispatch transaction", "error", err, "from", partition.Url)
			continue
		}
	}
}

func (x *Executor) getKeySignature(r *api.MessageRecord[messaging.Message], partition *protocol.PartitionSyntheticLedger, seq *messaging.SequencedMessage, anchor bool) (_ protocol.KeySignature, bad bool) {
	for _, set := range r.Signatures.Records {
		if set.Signatures == nil {
			x.logger.Error("Response to query-synth is missing the signatures", "from", partition.Url, "seq-num", seq.Number, "is-anchor", anchor)
			continue
		}

		for _, r := range set.Signatures.Records {
			msg, ok := r.Message.(*messaging.SignatureMessage)
			if !ok {
				continue
			}
			sig, ok := msg.Signature.(protocol.KeySignature)
			if !ok {
				x.logger.Error("Invalid signature in response to query-synth", "errors", errors.Conflict.WithFormat("expected key signature, got %T", msg.Signature), "from", partition.Url, "seq-num", seq.Number, "is-anchor", anchor, "hash", logging.AsHex(msg.Signature.Hash()), "signature", msg.Signature)
				return nil, true
			}
			return sig, false
		}
	}
	return nil, false
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

	// Send an anchor if a directory anchor was received and the flag is set
	return didAnchorDirectory && b.Executor.globals.Active.Globals.AnchorEmptyBlocks
}

func (x *Executor) prepareAnchor(block *Block) error {
	// Determine if an anchor should be sent
	if block.State.Anchor == nil {
		return nil
	}

	// Update the anchor ledger
	anchorLedger, err := database.UpdateAccount(block.Batch, x.Describe.AnchorPool(), func(ledger *protocol.AnchorLedger) error {
		ledger.MinorBlockSequenceNumber++
		if block.State.MajorBlock == nil {
			return nil
		}

		ledger.MajorBlockIndex = block.State.MajorBlock.Index
		ledger.MajorBlockTime = block.State.MajorBlock.Time

		if x.globals.Active.ExecutorVersion.V2VandenbergEnabled() {
			return nil
		}

		bvns := x.globals.Active.BvnNames()
		if x.globals.Active.ExecutorVersion.V2BaikonurEnabled() {
			// From Baikonur forward, sort this list so changes in the
			// implementation of BvnNames don't break it
			sort.Strings(bvns)

		} else {
			// Use the ordering of routes to sort the BVN list since that preserves
			// the order used prior to 1.3
			routes := map[string]int{}
			for i, r := range x.globals.Active.Routing.Routes {
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

	if !x.globals.Active.BvnExecutorVersion().V2VandenbergEnabled() {
		anchor.Updates = systemLedger.PendingUpdates
	}

	if block.State.MajorBlock != nil && !x.globals.Active.BvnExecutorVersion().V2VandenbergEnabled() {
		anchor.MakeMajorBlock = anchorLedger.MajorBlockIndex
		anchor.MakeMajorBlockTime = anchorLedger.MajorBlockTime
	}

	// Load the root chain
	rootChain, err := block.Batch.Account(x.Describe.Ledger()).RootChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load root chain: %w", err)
	}

	// TODO This is pretty inefficient; we're constructing a receipt for every
	// anchor. If we were more intelligent about it, we could send just the
	// Merkle state and a list of transactions, though we would need that for
	// the root chain and each anchor chain.

	anchorUrl := x.Describe.NodeUrl(protocol.AnchorPool)
	record := block.Batch.Account(anchorUrl)

	for _, received := range block.State.ReceivedAnchors {
		entry, err := indexing.LoadIndexEntryFromEnd(record.AnchorChain(received.Partition).Root().Index(), 1)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load last entry of %s intermediate anchor index chain: %w", received.Partition, err)
		}

		anchorChain, err := record.AnchorChain(received.Partition).Root().Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load %s intermediate anchor chain: %w", received.Partition, err)
		}

		rootReceipt, err := rootChain.Receipt(int64(entry.Anchor), rootChain.Height()-1)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("build receipt for entry %d (to %d) of the root chain: %w", entry.Anchor, rootChain.Height()-1, err)
		}

		anchorReceipt, err := anchorChain.Receipt(received.Index, int64(entry.Source))
		if err != nil {
			return nil, errors.UnknownError.WithFormat("build receipt for entry %d (to %d) of %s intermediate anchor chain: %w", received.Index, entry.Source, received.Partition, err)
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
	bvns := b.Executor.globals.Active.BvnNames()
	sort.Strings(bvns)

	/* ***** ACME burn (for credits) ***** */

	// If the active version is Vandenberg and ACME has been burnt
	if b.Executor.globals.Active.ExecutorVersion.V2VandenbergEnabled() &&
		b.State.AcmeBurnt.Sign() > 0 {
		body := new(protocol.SyntheticBurnTokens)
		body.Amount = b.State.AcmeBurnt
		txn := new(protocol.Transaction)
		txn.Header.Principal = protocol.AcmeUrl()
		txn.Body = body
		msg := new(messaging.TransactionMessage)
		msg.Transaction = txn

		b.State.Produced++

		err := b.Executor.produceSynthetic(b.Batch, []*ProducedMessage{{
			Destination: protocol.AcmeUrl(),
			Message:     msg,
		}}, b.Index)
		if err != nil {
			return errors.UnknownError.WithFormat("queue ACME burn: %w", err)
		}
	}

	/* ***** Network account updates (DN) ***** */

	// If the active version is Vandenberg, we're on the DN, and there's a
	// network update
	if b.Executor.globals.Active.ExecutorVersion.V2VandenbergEnabled() &&
		b.Executor.Describe.NetworkType == protocol.PartitionTypeDirectory &&
		len(b.State.NetworkUpdate) > 0 {
		for _, bvn := range bvns {
			b.State.Produced++
			err := b.Executor.produceSynthetic(b.Batch, []*ProducedMessage{{
				Destination: protocol.PartitionUrl(bvn),
				Message: &messaging.NetworkUpdate{
					Accounts: b.State.NetworkUpdate,
				},
			}}, b.Index)
			if err != nil {
				return errors.UnknownError.WithFormat("queue network update: %w", err)
			}
		}
	}

	/* ***** Major block notification ***** */

	// If the active version is Vandenberg, we're on the DN, and there's a major
	// block
	if b.Executor.globals.Active.ExecutorVersion.V2VandenbergEnabled() &&
		b.Executor.Describe.NetworkType == protocol.PartitionTypeDirectory &&
		b.State.MajorBlock != nil {
		for _, bvn := range bvns {
			b.State.Produced++
			err := b.Executor.produceSynthetic(b.Batch, []*ProducedMessage{{
				Destination: protocol.PartitionUrl(bvn),
				Message: &messaging.MakeMajorBlock{
					MajorBlockIndex: b.State.MajorBlock.Index,
					MajorBlockTime:  b.State.MajorBlock.Time,
					MinorBlockIndex: b.Index,
				},
			}}, b.Index)
			if err != nil {
				return errors.UnknownError.WithFormat("queue network update: %w", err)
			}
		}
	}

	/* ***** Did update version (BVN) ***** */

	// If the **pending** version is Vandenberg, we're on a BVN, and the version
	// is changing
	if b.Executor.globals.Pending.ExecutorVersion.V2VandenbergEnabled() &&
		b.Executor.Describe.NetworkType != protocol.PartitionTypeDirectory &&
		b.Executor.globals.Pending.ExecutorVersion != b.Executor.globals.Active.ExecutorVersion {
		b.State.Produced++
		err := b.Executor.produceSynthetic(b.Batch, []*ProducedMessage{{
			Destination: protocol.DnUrl(),
			Message: &messaging.DidUpdateExecutorVersion{
				Partition: b.Executor.Describe.PartitionId,
				Version:   b.Executor.globals.Pending.ExecutorVersion,
			},
		}}, b.Index)
		if err != nil {
			return errors.UnknownError.WithFormat("queue update notification: %w", err)
		}
	}

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

	if !x.globals.Active.ExecutorVersion.V2VandenbergEnabled() {
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
