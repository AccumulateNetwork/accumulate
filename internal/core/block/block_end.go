// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
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
		return errors.Format(errors.StatusUnknownError, "load synthetic ledger: %w", err)
	}
	var anchorLedger *protocol.AnchorLedger
	err = block.Batch.Account(m.Describe.AnchorPool()).GetStateAs(&anchorLedger)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load synthetic ledger: %w", err)
	}
	m.Background(func() { m.requestMissingSyntheticTransactions(block.Index, synthLedger, anchorLedger) })

	// Update active globals
	if !m.isGenesis && !m.globals.Active.Equal(&m.globals.Pending) {
		err = m.EventBus.Publish(events.WillChangeGlobals{
			New: &m.globals.Pending,
			Old: &m.globals.Active,
		})
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "publish globals update: %w", err)
		}
		m.globals.Active = *m.globals.Pending.Copy()
	}

	// Determine if an anchor should be sent
	m.shouldPrepareAnchor(block)

	// Do nothing if the block is empty
	if block.State.Empty() {
		return nil
	}

	// Load the ledger
	ledgerUrl := m.Describe.NodeUrl(protocol.Ledger)
	ledger := block.Batch.Account(ledgerUrl)
	var ledgerState *protocol.SystemLedger
	err = ledger.GetStateAs(&ledgerState)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load system ledger: %w", err)
	}

	m.logger.Debug("Committing",
		"module", "block",
		"height", block.Index,
		"delivered", block.State.Delivered,
		"signed", block.State.Signed,
		"updated", len(block.State.ChainUpdates.Entries),
		"produced", len(block.State.ProducedTxns))
	t := time.Now()

	// Load the main chain of the minor root
	rootChain, err := ledger.RootChain().Get()
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load root chain: %w", err)
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
			return errors.Format(errors.StatusUnknownError, "resolve chain %v of %v: %w", entry.Chain, entry.Account, err)
		}
		chain2, err := chain.Get()
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "load chain %v of %v: %w", entry.Chain, entry.Account, err)
		}
		hash, err := chain2.Entry(int64(entry.Index))
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "load entry %d of chain %v of %v: %w", entry.Index, entry.Chain, entry.Account, err)
		}
		u.Txns = append(u.Txns, hash)

		// Only anchor each chain once
		if anchored {
			continue
		}

		// Anchor and index the chain
		u.IndexIndex, u.DidIndex, err = addChainAnchor(rootChain, chain, block.Index)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "add anchor to root chain: %w", err)
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
		return errors.Format(errors.StatusUnknownError, "store block ledger: %w", err)
	}

	// Add the synthetic transaction chain to the root chain
	var synthIndexIndex uint64
	var synthAnchorIndex uint64
	if len(block.State.ProducedTxns) > 0 {
		synthAnchorIndex = uint64(rootChain.Height())
		synthIndexIndex, err = m.anchorSynthChain(block, rootChain)
		if err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}
	}

	// Write the updated ledger
	err = ledger.PutState(ledgerState)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "store system ledger: %w", err)
	}

	// Index the root chain
	rootIndexIndex, err := addIndexChainEntry(ledger.RootChain().Index(), &protocol.IndexEntry{
		Source:     uint64(rootChain.Height() - 1),
		BlockIndex: uint64(block.Index),
		BlockTime:  &block.Time,
	})
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "add root index chain entry: %w", err)
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
				return errors.Format(errors.StatusUnknownError, "store transaction chain index: %w", err)
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
			return errors.Format(errors.StatusUnknownError, "store transaction chain index: %w", err)
		}
	}

	if len(block.State.ProducedTxns) > 0 {
		// Build synthetic receipts on Directory nodes
		if m.Describe.NetworkType == config.Directory {
			err = m.createLocalDNReceipt(block, rootChain, synthAnchorIndex)
			if err != nil {
				return errors.Wrap(errors.StatusUnknownError, err)
			}
		}

		// Build synthetic receipts
		err = m.buildSynthReceipt(block.Batch, block.State.ProducedTxns, rootChain.Height()-1, int64(synthAnchorIndex))
		if err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}
	}

	// Update major index chains if it's a major block
	err = m.updateMajorIndexChains(block, rootIndexIndex)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	// Check if an anchor needs to be sent
	err = m.prepareAnchor(block)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	m.logger.Debug("Committed", "module", "block", "height", block.Index, "duration", time.Since(t))
	return nil
}

func (m *Executor) createLocalDNReceipt(block *Block, rootChain *database.Chain, synthAnchorIndex uint64) error {
	rootReceipt, err := rootChain.Receipt(int64(synthAnchorIndex), rootChain.Height()-1)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "build root chain receipt: %w", err)
	}

	synthChain, err := block.Batch.Account(m.Describe.Synthetic()).MainChain().Get()
	if err != nil {
		return fmt.Errorf("unable to load synthetic transaction chain: %w", err)
	}

	height := synthChain.Height()
	offset := height - int64(len(block.State.ProducedTxns))
	for i, txn := range block.State.ProducedTxns {
		if txn.Body.Type().IsSystem() {
			// Do not generate a receipt for the anchor
			continue
		}

		synthReceipt, err := synthChain.Receipt(offset+int64(i), height-1)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "build synth chain receipt: %w", err)
		}

		receipt, err := synthReceipt.Combine(rootReceipt)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "combine receipts: %w", err)
		}

		// This should be the second signature (SyntheticSignature should be first)
		sig := new(protocol.ReceiptSignature)
		sig.SourceNetwork = m.Describe.NodeUrl()
		sig.TransactionHash = *(*[32]byte)(txn.GetHash())
		sig.Proof = *receipt
		_, err = block.Batch.Transaction(txn.GetHash()).AddSystemSignature(&m.Describe, sig)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "store signature: %w", err)
		}
	}
	return nil
}

// anchorSynthChain anchors the synthetic transaction chain.
func (m *Executor) anchorSynthChain(block *Block, rootChain *database.Chain) (indexIndex uint64, err error) {
	url := m.Describe.Synthetic()
	indexIndex, _, err = addChainAnchor(rootChain, block.Batch.Account(url).MainChain(), block.Index)
	if err != nil {
		return 0, errors.Wrap(errors.StatusUnknownError, err)
	}

	err = block.Batch.SystemData(m.Describe.PartitionId).SyntheticIndexIndex(block.Index).Put(indexIndex)
	if err != nil {
		return 0, errors.Format(errors.StatusUnknownError, "store synthetic transaction index index for block: %w", err)
	}

	block.State.ChainUpdates.DidUpdateChain(&protocol.BlockEntry{
		Account: url,
		Chain:   protocol.MainChain,
	})

	return indexIndex, nil
}

func (x *Executor) requestMissingSyntheticTransactions(blockIndex uint64, synthLedger *protocol.SyntheticLedger, anchorLedger *protocol.AnchorLedger) {
	batch := x.db.Begin(false)
	defer batch.Discard()

	// Setup
	wg := new(sync.WaitGroup)
	dispatcher := newDispatcher(x.ExecutorOptions)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// For each partition
	var pending []*url.TxID
	for _, partition := range synthLedger.Sequence {
		pending = append(pending, x.requestMissingTransactionsFromPartition(ctx, wg, dispatcher, partition, false)...)
	}
	for _, partition := range anchorLedger.Sequence {
		pending = append(pending, x.requestMissingTransactionsFromPartition(ctx, wg, dispatcher, partition, true)...)
	}

	// See if we can get the anchors we need for pending synthetic transactions.
	// https://accumulate.atlassian.net/browse/AC-1860
	if x.Describe.NetworkType != config.Directory && len(pending) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			x.requestMissingAnchors(ctx, batch, dispatcher, blockIndex, pending)
		}()
	}

	wg.Wait()
	for err := range dispatcher.Send(ctx) {
		x.logger.Error("Failed to dispatch missing synthetic transactions", "error", err)
	}
}

func (x *Executor) requestMissingTransactionsFromPartition(ctx context.Context, wg *sync.WaitGroup, dispatcher *dispatcher, partition *protocol.PartitionSyntheticLedger, anchor bool) []*url.TxID {
	var pending []*url.TxID
	// Get the partition ID
	id, ok := protocol.ParsePartitionUrl(partition.Url)
	if !ok {
		// If this happens we're kind of screwed
		panic(errors.Format(errors.StatusInternalError, "synthetic ledger has an invalid partition URL: %v", partition.Url))
	}

	// For each pending synthetic transaction
	var batch jsonrpc2.BatchRequest
	for i, txid := range partition.Pending {
		// If we know the ID we must have a local copy (so we don't need to
		// fetch it)
		if txid != nil {
			pending = append(pending, txid)
			continue
		}

		seqNum := partition.Delivered + uint64(i) + 1
		var message = "Missing synthetic transaction"
		if anchor {
			message = "Missing anchor transaction"
		}
		x.logger.Info(message, "seq-num", seqNum, "source", partition.Url)

		// Request the transaction by sequence number
		batch = append(batch, jsonrpc2.Request{
			ID:     i + 1,
			Method: "query-synth",
			Params: &api.SyntheticTransactionRequest{
				Source:         partition.Url,
				Destination:    x.Describe.NodeUrl(),
				SequenceNumber: seqNum,
				Anchor:         anchor,
			},
		})
		if x.BatchReplayLimit > 0 && len(batch) == x.BatchReplayLimit {
			break
		}
	}

	if len(batch) == 0 {
		return pending
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		// Send the requests
		var resp []*api.TransactionQueryResponse
		err := x.Router.RequestAPIv2(ctx, id, "", batch, &resp)
		if err != nil {
			x.logger.Error("Failed to request synthetic transactions", "error", err, "from", partition.Url)
			return
		}

		// Sanity check
		if len(resp) != len(batch) {
			x.logger.Error("Bad response to query-synth: the number of responses does not match the number of requests", "from", partition.Url, "want", len(batch), "got", len(resp))
			return
		}

		// Broadcast each transaction locally
		for i, resp := range resp {
			req := batch[i].Params.(*api.SyntheticTransactionRequest)

			// Sanity check: the response includes a transaction
			if resp.Transaction == nil {
				x.logger.Error("Response to query-synth is missing the transaction", "from", partition.Url, "seq-num", req.SequenceNumber, "is-anchor", req.Anchor)
				continue
			}

			// Put the synthetic signature first
			var gotSynth, gotReceipt, gotKey, bad bool
			for i, signature := range resp.Signatures {
				h := signature.GetTransactionHash()
				if !bytes.Equal(h[:], resp.Transaction.GetHash()) {
					x.logger.Error("Signature from query-synth does not match the transaction hash", "from", partition.Url, "seq-num", req.SequenceNumber, "is-anchor", req.Anchor, "hash", logging.AsHex(resp.Transaction.GetHash()).Slice(0, 4), "signature", signature)
					bad = true
					continue
				}

				switch signature.(type) {
				case *protocol.PartitionSignature:
					gotSynth = true
					resp.Signatures[0], resp.Signatures[i] = resp.Signatures[i], resp.Signatures[0]
				case *protocol.ReceiptSignature:
					gotReceipt = true
				case *protocol.ED25519Signature:
					gotKey = true
				}
			}

			var missing []string
			if !gotSynth {
				missing = append(missing, "synthetic")
			}
			if !gotReceipt && !anchor {
				missing = append(missing, "receipt")
			}
			if !gotKey {
				missing = append(missing, "key")
			}
			if len(missing) > 0 {
				err := fmt.Errorf("missing %s signature(s)", strings.Join(missing, ", "))
				x.logger.Error("Invalid synthetic transaction", "error", err, "hash", logging.AsHex(resp.Transaction.GetHash()).Slice(0, 4), "type", resp.Transaction.Body.Type())
				bad = true
			}

			if bad {
				continue
			}

			err = dispatcher.BroadcastTxLocal(ctx, &protocol.Envelope{
				Signatures:  resp.Signatures,
				Transaction: []*protocol.Transaction{resp.Transaction},
			})
			if err != nil {
				x.logger.Error("Failed to dispatch synthetic transaction", "error", err, "from", partition.Url)
				continue
			}
		}
	}()

	return pending
}

func (x *Executor) requestMissingAnchors(ctx context.Context, batch *database.Batch, dispatcher *dispatcher, blockIndex uint64, pending []*url.TxID) {
	anchors := map[[32]byte][]*url.TxID{}
	source := map[*url.TxID]*url.URL{}
	for _, txid := range pending {
		h := txid.Hash()
		status, err := batch.Transaction(h[:]).GetStatus()
		if err != nil {
			x.logger.Error("Error loading synthetic transaction status", "error", err, "hash", logging.AsHex(txid.Hash()).Slice(0, 4))
			continue
		}
		// Skip if...
		if status.GotDirectoryReceipt || //       We have a full receipt
			status.Code == 0 || //                We haven't received the transaction (synthetic transaction from a BVN to itself)
			status.Proof == nil || //             It doesn't have a proof (because it's an anchor)
			status.Received+10 < blockIndex || // It was received less than 10 blocks ago
			false {
			continue
		}
		a := *(*[32]byte)(status.Proof.Anchor)
		anchors[a] = append(anchors[a], txid)
		source[txid] = status.SourceNetwork
		if x.BatchReplayLimit > 0 && len(anchors) == x.BatchReplayLimit {
			break
		}
	}
	if len(anchors) == 0 {
		return
	}

	var jbatch jsonrpc2.BatchRequest
	for anchor := range anchors {
		jbatch = append(jbatch, jsonrpc2.Request{
			ID:     1,
			Method: "query",
			Params: &api.GeneralQuery{
				UrlQuery: api.UrlQuery{
					Url: protocol.DnUrl().JoinPath(protocol.AnchorPool).WithFragment(fmt.Sprintf("anchor/%x", anchor)),
				},
			},
		})
	}

	// Send the requests
	var resp []*api.ChainQueryResponse
	err := x.Router.RequestAPIv2(ctx, protocol.Directory, "", jbatch, &resp)
	if err != nil {
		// If an individual request failed, ignore it
		var berr jsonrpc2.BatchError
		if !errors.As(err, &berr) {
			x.logger.Error("Failed to request anchors", "error", err)
			return
		}
	}

	var sigs []protocol.Signature
	for _, resp := range resp {
		if resp == nil || resp.Receipt == nil || resp.Receipt.Error != "" {
			continue
		}

		for _, txid := range anchors[*(*[32]byte)(resp.Receipt.Proof.Start)] {
			sig := new(protocol.ReceiptSignature)
			sig.SourceNetwork = protocol.DnUrl()
			sig.Proof = resp.Receipt.Proof
			sig.TransactionHash = txid.Hash()
			sigs = append(sigs, sig)
		}
	}

	if len(sigs) == 0 {
		return
	}

	err = dispatcher.BroadcastTxLocal(ctx, &protocol.Envelope{Signatures: sigs})
	if err != nil {
		x.logger.Error("Failed to dispatch receipts", "error", err)
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
		return errors.Format(errors.StatusUnknownError, "load anchor ledger main chain: %w", err)
	}

	_, err = addIndexChainEntry(account.MajorBlockChain(), &protocol.IndexEntry{
		Source:         uint64(mainChain.Height() - 1),
		RootIndexIndex: rootIndexIndex,
		BlockIndex:     block.State.MakeMajorBlock,
		BlockTime:      &block.State.MakeMajorBlockTime,
	})
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "add anchor ledger index chain entry: %w", err)
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
	if len(block.State.ProducedTxns) > 0 {
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
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	// Update the system ledger
	_, err = database.UpdateAccount(block.Batch, x.Describe.Ledger(), func(ledger *protocol.SystemLedger) error {
		if x.Describe.NetworkType == config.Directory {
			ledger.Anchor, err = x.buildDirectoryAnchor(block, ledger, anchorLedger)
		} else {
			ledger.Anchor, err = x.buildPartitionAnchor(block, ledger)
		}
		return errors.Wrap(errors.StatusUnknownError, err)
	})
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	return errors.Wrap(errors.StatusUnknownError, err)
}

func (x *Executor) buildDirectoryAnchor(block *Block, systemLedger *protocol.SystemLedger, anchorLedger *protocol.AnchorLedger) (*protocol.DirectoryAnchor, error) {
	// Do not populate the root chain index, root chain anchor, or state tree
	// anchor. Those cannot be populated until the block is complete, thus they
	// cannot be populated until the next block starts.
	anchor := new(protocol.DirectoryAnchor)
	anchor.Source = x.Describe.NodeUrl()
	anchor.MinorBlockIndex = uint64(block.Index)
	anchor.MajorBlockIndex = block.State.MakeMajorBlock
	anchor.Updates = systemLedger.PendingUpdates
	if block.State.Anchor.ShouldOpenMajorBlock {
		anchor.MakeMajorBlock = anchorLedger.MajorBlockIndex
		anchor.MakeMajorBlockTime = anchorLedger.MajorBlockTime
	}

	// Load the root chain
	rootChain, err := block.Batch.Account(x.Describe.Ledger()).RootChain().Get()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load root chain: %w", err)
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
			return nil, errors.Format(errors.StatusUnknownError, "load last entry of %s intermediate anchor index chain: %w", received.Partition, err)
		}

		anchorChain, err := record.AnchorChain(received.Partition).Root().Get()
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "load %s intermediate anchor chain: %w", received.Partition, err)
		}

		rootReceipt, err := rootChain.Receipt(int64(entry.Anchor), rootChain.Height()-1)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "build receipt for entry %d (to %d) of the root chain: %w", entry.Anchor, rootChain.Height()-1, err)
		}

		anchorReceipt, err := anchorChain.Receipt(received.Index, int64(entry.Source))
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "build receipt for entry %d (to %d) of %s intermediate anchor chain: %w", received.Index, entry.Source, received.Partition, err)
		}

		receipt := new(protocol.PartitionAnchorReceipt)
		receipt.Anchor = received.Body.GetPartitionAnchor()
		receipt.RootChainReceipt, err = anchorReceipt.Combine(rootReceipt)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "combine receipt for entry %d of %s intermediate anchor chain: %w", received.Index, received.Partition, err)
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
	anchor.MinorBlockIndex = uint64(block.Index)
	anchor.MajorBlockIndex = block.State.MakeMajorBlock
	anchor.AcmeBurnt = ledger.AcmeBurnt
	return anchor, nil
}
