// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// EndBlock implements ./Chain
func (m *Executor) EndBlock(block *Block) error {
	r := m.BlockTimers.Start(BlockTimerTypeEndBlock)
	defer m.BlockTimers.Stop(r)

	// Check for missing synthetic transactions. Load the ledger synchronously,
	// request transactions asynchronously.
	var synthLedger *protocol.SyntheticLedger
	err := block.Batch.Account(m.Describe.Synthetic()).GetStateAs(&synthLedger)
	if err != nil {
		return errors.UnknownError.WithFormat("load synthetic ledger: %w", err)
	}
	var anchorLedger *protocol.AnchorLedger
	err = block.Batch.Account(m.Describe.AnchorPool()).GetStateAs(&anchorLedger)
	if err != nil {
		return errors.UnknownError.WithFormat("load synthetic ledger: %w", err)
	}
	m.BackgroundTaskLauncher(func() { m.requestMissingSyntheticTransactions(block.Index, synthLedger, anchorLedger) })

	// Update active globals
	if !m.isGenesis && !m.globals.Active.Equal(&m.globals.Pending) {
		err = m.EventBus.Publish(events.WillChangeGlobals{
			New: &m.globals.Pending,
			Old: &m.globals.Active,
		})
		if err != nil {
			return errors.UnknownError.WithFormat("publish globals update: %w", err)
		}
		m.globals.Active = *m.globals.Pending.Copy()
	}

	// Determine if an anchor should be sent
	m.shouldPrepareAnchor(block)

	// Do nothing if the block is empty
	if block.State.Empty() {
		return nil
	}

	m.logger.Debug("Committing",
		"module", "block",
		"height", block.Index,
		"delivered", block.State.Delivered,
		"signed", block.State.Signed,
		"updated", len(block.State.ChainUpdates.Entries),
		"produced", block.State.Produced)
	t := time.Now()

	// Load the main chain of the minor root
	ledgerUrl := m.Describe.NodeUrl(protocol.Ledger)
	ledger := block.Batch.Account(ledgerUrl)
	rootChain, err := ledger.RootChain().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load root chain: %w", err)
	}

	if m.globals.Active.ExecutorVersion.SignatureAnchoringEnabled() {
		// Overwrite the state's chain update list with one derived directly
		// from the database
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
			return errors.UnknownError.WithFormat("resolve chain %v of %v: %w", entry.Chain, entry.Account, err)
		}
		chain2, err := chain.Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load chain %v of %v: %w", entry.Chain, entry.Account, err)
		}
		hash, err := chain2.Entry(int64(entry.Index))
		if err != nil {
			return errors.UnknownError.WithFormat("load entry %d of chain %v of %v: %w", entry.Index, entry.Chain, entry.Account, err)
		}
		u.Txns = append(u.Txns, hash)

		// Only anchor each chain once
		if anchored {
			continue
		}

		// Anchor and index the chain
		u.IndexIndex, u.DidIndex, err = addChainAnchor(rootChain, chain, block.Index)
		if err != nil {
			return errors.UnknownError.WithFormat("add anchor to root chain: %w", err)
		}
	}

	// Record the block entries
	bl := new(protocol.BlockLedger)
	bl.Url = m.Describe.Ledger().JoinPath(strconv.FormatUint(block.Index, 10))
	bl.Index = block.Index
	bl.Time = block.Time
	bl.Entries = block.State.ChainUpdates.Entries
	err = block.Batch.Account(bl.Url).Main().Put(bl)
	if err != nil {
		return errors.UnknownError.WithFormat("store block ledger: %w", err)
	}

	// Add the synthetic transaction chain to the root chain
	var synthIndexIndex uint64
	if block.State.Produced > 0 {
		synthIndexIndex, err = m.anchorSynthChain(block, rootChain)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	// Index the root chain
	rootIndexIndex, err := addIndexChainEntry(ledger.RootChain().Index(), &protocol.IndexEntry{
		Source:     uint64(rootChain.Height() - 1),
		BlockIndex: block.Index,
		BlockTime:  &block.Time,
	})
	if err != nil {
		return errors.UnknownError.WithFormat("add root index chain entry: %w", err)
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
				return errors.UnknownError.WithFormat("store transaction chain index: %w", err)
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
			return errors.UnknownError.WithFormat("store transaction chain index: %w", err)
		}
	}

	// Update major index chains if it's a major block
	err = m.updateMajorIndexChains(block, rootIndexIndex)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Check if an anchor needs to be sent
	err = m.prepareAnchor(block)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	m.logger.Debug("Committed", "module", "block", "height", block.Index, "duration", time.Since(t))
	return nil
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
	wg := new(sync.WaitGroup)
	dispatcher := x.NewDispatcher()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// For each partition
	for _, partition := range synthLedger.Sequence {
		x.requestMissingTransactionsFromPartition(ctx, wg, dispatcher, partition, false)
	}
	for _, partition := range anchorLedger.Sequence {
		x.requestMissingTransactionsFromPartition(ctx, wg, dispatcher, partition, true)
	}

	wg.Wait()
	for err := range dispatcher.Send(ctx) {
		switch err := err.(type) {
		case protocol.TransactionStatusError:
			x.logger.Error("Failed to dispatch transactions", "error", err, "stack", err.TransactionStatus.Error.PrintFullCallstack(), "txid", err.TxID)
		default:
			x.logger.Error("Failed to dispatch transactions", "error", err, "stack", fmt.Sprintf("%+v\n", err))
		}
	}
}

func (x *Executor) requestMissingTransactionsFromPartition(ctx context.Context, wg *sync.WaitGroup, dispatcher Dispatcher, partition *protocol.PartitionSyntheticLedger, anchor bool) {
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
		resp, err := x.Sequencer.Sequence(ctx, src, dest, seqNum)
		if err != nil {
			x.logger.Error("Failed to request sequenced transaction", "error", err, "from", src, "seq-num", seqNum)
			continue
		}

		// Sanity check: the response includes a transaction
		if resp.Transaction == nil {
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
			if resp.Status == nil {
				x.logger.Error("Response to query-synth is missing the status", "from", partition.Url, "seq-num", seqNum, "is-anchor", anchor)
				continue
			}
			if resp.Status.Proof == nil {
				x.logger.Error("Response to query-synth is missing the proof", "from", partition.Url, "seq-num", seqNum, "is-anchor", anchor)
				continue
			}
		}

		seq := &messaging.SequencedMessage{
			Message: &messaging.UserTransaction{
				Transaction: resp.Transaction,
			},
			Source:      resp.Sequence.Source,
			Destination: resp.Sequence.Destination,
			Number:      resp.Sequence.Number,
		}

		var messages []messaging.Message
		if anchor {
			messages = []messaging.Message{seq}
		} else {
			messages = []messaging.Message{
				&messaging.SyntheticMessage{
					Message: seq,
					Proof: &protocol.AnnotatedReceipt{
						Receipt: resp.Status.Proof,
						Anchor: &protocol.AnchorMetadata{
							Account: protocol.DnUrl(),
						},
					},
				},
			}
		}

		var gotKey, bad bool
		for _, signature := range resp.Signatures.Records {
			h := signature.TxID.Hash()
			if !bytes.Equal(h[:], resp.Transaction.GetHash()) {
				x.logger.Error("Signature from query-synth does not match the transaction hash", "from", partition.Url, "seq-num", seqNum, "is-anchor", anchor, "txid", signature.TxID, "signature", signature)
				bad = true
				continue
			}

			set, ok := signature.Signature.(*protocol.SignatureSet)
			if !ok {
				x.logger.Error("Invalid signature in response to query-synth", "errors", errors.Conflict.WithFormat("expected %T, got %T", (*protocol.SignatureSet)(nil), signature.Signature), "from", partition.Url, "seq-num", seqNum, "is-anchor", anchor, "txid", signature.TxID, "signature", signature)
				bad = true
				continue
			}

			for _, signature := range set.Signatures {
				keySig, ok := signature.(protocol.KeySignature)
				if !ok {
					x.logger.Error("Invalid signature in response to query-synth", "errors", errors.Conflict.WithFormat("expected key signature, got %T", signature), "from", partition.Url, "seq-num", seqNum, "is-anchor", anchor, "hash", logging.AsHex(signature.Hash()), "signature", signature)
					bad = true
					continue
				}
				gotKey = true
				messages = append(messages, &messaging.ValidatorSignature{
					Signature: keySig,
					Source:    partition.Url,
				})
			}

		}

		if !gotKey {
			x.logger.Error("Invalid synthetic transaction", "error", "missing key signature", "hash", logging.AsHex(resp.Transaction.GetHash()).Slice(0, 4), "type", resp.Transaction.Body.Type())
			bad = true
		}
		if bad {
			continue
		}

		err = dispatcher.Submit(ctx, dest, &messaging.Envelope{Messages: messages})
		if err != nil {
			x.logger.Error("Failed to dispatch synthetic transaction", "error", err, "from", partition.Url)
			continue
		}
	}
}

// updateMajorIndexChains updates major index chains.
func (x *Executor) updateMajorIndexChains(block *Block, rootIndexIndex uint64) error {
	if block.State.MakeMajorBlock == 0 {
		return nil
	}

	// Load the chain
	account := block.Batch.Account(x.Describe.AnchorPool())
	mainChain, err := account.MainChain().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load anchor ledger main chain: %w", err)
	}

	_, err = addIndexChainEntry(account.MajorBlockChain(), &protocol.IndexEntry{
		Source:         uint64(mainChain.Height() - 1),
		RootIndexIndex: rootIndexIndex,
		BlockIndex:     block.State.MakeMajorBlock,
		BlockTime:      &block.State.MakeMajorBlockTime,
	})
	if err != nil {
		return errors.UnknownError.WithFormat("add anchor ledger index chain entry: %w", err)
	}

	return nil
}

func (x *Executor) shouldPrepareAnchor(block *Block) {
	openMajorBlock, openMajorBlockTime := x.shouldOpenMajorBlock(block.Batch, block.Time.Add(time.Second))
	sendAnchor := openMajorBlock || x.shouldSendAnchor(block)
	if sendAnchor {
		block.State.Anchor = &BlockAnchorState{
			ShouldOpenMajorBlock: openMajorBlock,
			OpenMajorBlockTime:   openMajorBlockTime,
		}
	}
}

func (x *Executor) shouldOpenMajorBlock(batch *database.Batch, blockTime time.Time) (bool, time.Time) {
	// Only the directory network can open a major block
	if x.Describe.NetworkType != config.Directory {
		return false, time.Time{}
	}

	// Only when majorBlockSchedule is initialized we can open a major block. (not when doing replayBlocks)
	if x.ExecutorOptions.MajorBlockScheduler == nil || !x.ExecutorOptions.MajorBlockScheduler.IsInitialized() {
		return false, time.Time{}
	}

	blockTimeUTC := blockTime.UTC()
	nextBlockTime := x.ExecutorOptions.MajorBlockScheduler.GetNextMajorBlockTime(blockTime)
	if blockTimeUTC.IsZero() || blockTimeUTC.Before(nextBlockTime) {
		return false, time.Time{}
	}

	return true, blockTimeUTC
}

func (x *Executor) shouldSendAnchor(block *Block) bool {
	// Did we make a major block?
	if block.State.MakeMajorBlock > 0 {
		return true
	}

	// Did we produce synthetic transactions?
	if block.State.Produced > 0 {
		return true
	}

	var didUpdateOther, didAnchorPartition, didAnchorDirectory bool
	anchor := block.Batch.Account(x.Describe.AnchorPool())
	for _, c := range block.State.ChainUpdates.Entries {
		// The system ledger is always updated
		if c.Account.Equal(x.Describe.Ledger()) {
			continue
		}

		// Was some other account updated?
		if !c.Account.Equal(x.Describe.AnchorPool()) {
			didUpdateOther = true
			continue
		}

		// Check if a partition anchor was received
		chain, err := anchor.ChainByName(c.Chain)
		if err != nil {
			x.logger.Error("Failed to get chain by name", "error", err, "name", c.Chain)
			continue
		}

		partition, ok := chain.Key(3).(string)
		if chain.Key(2) != "AnchorChain" || !ok {
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
	return didAnchorDirectory && x.globals.Active.Globals.AnchorEmptyBlocks
}

func (x *Executor) prepareAnchor(block *Block) error {
	// Determine if an anchor should be sent
	if block.State.Anchor == nil {
		return nil
	}

	// Update the anchor ledger
	anchorLedger, err := database.UpdateAccount(block.Batch, x.Describe.AnchorPool(), func(ledger *protocol.AnchorLedger) error {
		ledger.MinorBlockSequenceNumber++
		if !block.State.Anchor.ShouldOpenMajorBlock {
			return nil
		}

		bvns := x.Describe.Network.GetBvnNames()
		ledger.MajorBlockIndex++
		ledger.MajorBlockTime = block.State.Anchor.OpenMajorBlockTime
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
		if x.Describe.NetworkType == config.Directory {
			ledger.Anchor, err = x.buildDirectoryAnchor(block, ledger, anchorLedger)
		} else {
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
	anchor.Updates = systemLedger.PendingUpdates
	if block.State.Anchor.ShouldOpenMajorBlock {
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

func (x *Executor) buildPartitionAnchor(block *Block, ledger *protocol.SystemLedger) (*protocol.BlockValidatorAnchor, error) {
	// Do not populate the root chain index, root chain anchor, or state tree
	// anchor. Those cannot be populated until the block is complete, thus they
	// cannot be populated until the next block starts.
	anchor := new(protocol.BlockValidatorAnchor)
	anchor.Source = x.Describe.NodeUrl()
	anchor.MinorBlockIndex = block.Index
	anchor.MajorBlockIndex = block.State.MakeMajorBlock
	anchor.AcmeBurnt = ledger.AcmeBurnt
	return anchor, nil
}
