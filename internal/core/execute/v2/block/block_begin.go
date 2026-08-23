// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/crosschain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	dagconfig "gitlab.com/accumulatenetwork/accumulate/pkg/consensus/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Begin constructs a [Block] and calls [Executor.BeginBlock].
func (x *Executor) Begin(params execute.BlockParams) (_ execute.Block, err error) {
	block := new(Block)
	block.BlockParams = params
	block.Executor = x
	block.Batch = x.Database.Begin(true)

	defer func() {
		if err != nil {
			block.Batch.Discard()
		}
	}()

	err = x.EventBus.Publish(execute.WillBeginBlock{BlockParams: params})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	//clear the timers
	x.BlockTimers.Reset()

	r := x.BlockTimers.Start(BlockTimerTypeBeginBlock)
	defer x.BlockTimers.Stop(r)

	x.logger.Debug("Begin block", "module", "block", "height", block.Index, "leader", block.IsLeader, "time", block.Time)

	// Get the previous block's root hash (before any changes are made)
	block.State.PreviousStateHash, err = block.Batch.GetBptRootHash()
	if err != nil {
		return nil, err
	}

	// Finalize the previous block
	err = x.finalizeBlock(block)
	if err != nil {
		return nil, err
	}

	errs := x.mainDispatcher.Send(context.Background())
	x.BackgroundTaskLauncher(func() {
		for err := range errs {
			switch err := err.(type) {
			case protocol.TransactionStatusError:
				x.logger.Error("Failed to dispatch transactions", "block", block.Index, "error", err, "stack", err.TransactionStatus.Error.PrintFullCallstack(), "txid", err.TxID)
			default:
				x.logger.Error("Failed to dispatch transactions", "block", block.Index, "error", err, "stack", fmt.Sprintf("%+v\n", err))
			}
		}
	})

	// Load the ledger state
	ledger := block.Batch.Account(x.Describe.NodeUrl(protocol.Ledger))
	var ledgerState *protocol.SystemLedger
	err = ledger.Main().GetAs(&ledgerState)
	switch {
	case err == nil:
		// Make sure the block index is increasing
		if ledgerState.Index >= block.Index {
			panic(fmt.Errorf("current height is %d but the next block height is %d", ledgerState.Index, block.Index))
		}

	case x.isGenesis && errors.Is(err, storage.ErrNotFound):
		// OK

	default:
		return nil, fmt.Errorf("cannot load ledger: %w", err)
	}
	lastBlockWasEmpty := ledgerState.Index < block.Index-1

	// Reset transient values
	ledgerState.Index = block.Index
	ledgerState.Timestamp = block.Time
	ledgerState.PendingUpdates = nil
	ledgerState.AcmeBurnt = *big.NewInt(0)
	ledgerState.Anchor = nil

	err = ledger.Main().Put(ledgerState)
	if err != nil {
		return nil, fmt.Errorf("cannot write ledger: %w", err)
	}

	if !lastBlockWasEmpty {
		// Store votes from previous block, choosing to marshal as json to make it
		// easily viewable by explorers
		err = x.captureValueAsDataEntry(block.Batch, protocol.Votes, block.CommitInfo)
		if err != nil {
			x.logger.Error("Error processing internal vote transaction", "error", err)
		}

		// Capture evidence of maleficence if any occurred
		err = x.captureValueAsDataEntry(block.Batch, protocol.Evidence, block.Evidence)
		if err != nil {
			x.logger.Error("Error processing internal vote transaction", "error", err)
		}
	}

	// Deliver everything the previous block queued — local synthetics and
	// deferred cascades — before any of this block's own messages (#4146).
	err = block.drainDeliveryQueues()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("drain delivery queues: %w", err)
	}

	return block, nil
}

func (x *Executor) captureValueAsDataEntry(batch *database.Batch, internalAccountPath string, value interface{}) error {
	if value == nil {
		return nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return errors.UnknownError.WithFormat("cannot marshal value as json: %w", err)
	}

	wd := protocol.SystemWriteData{}
	if x.globals.Active.ExecutorVersion.DoubleHashEntriesEnabled() {
		wd.Entry = &protocol.DoubleHashDataEntry{Data: [][]byte{data}}
	} else {
		wd.Entry = &protocol.AccumulateDataEntry{Data: [][]byte{data}}
	}
	dataAccountUrl := x.Describe.NodeUrl(internalAccountPath)

	var signer protocol.Signer
	signerUrl := x.Describe.OperatorsPage()
	err = batch.Account(signerUrl).Main().GetAs(&signer)
	if err != nil {
		return err
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = x.Describe.NodeUrl()
	txn.Body = &wd
	txn.Header.Initiator = signerUrl.AccountID32()

	var da *protocol.DataAccount
	va := batch.Account(dataAccountUrl)
	err = va.Main().GetAs(&da)
	if err != nil {
		return err
	}

	// Add data index entry
	err = indexing.Data(batch, dataAccountUrl).Put(wd.Entry.Hash(), txn.GetHash())
	if err != nil {
		return fmt.Errorf("failed to add entry to data index of %q: %v", dataAccountUrl, err)
	}

	// Add TX to main chain
	var st chain.ProcessTransactionState
	err = st.ChainUpdates.AddChainEntry(batch, batch.Account(dataAccountUrl).MainChain(), txn.GetHash(), 0, 0)
	if err != nil {
		return err
	}

	err = putMessageWithStatus(batch,
		&messaging.TransactionMessage{Transaction: txn},
		&protocol.TransactionStatus{Code: errors.Delivered})
	if err != nil {
		return err
	}

	return nil
}

// finalizeBlock builds the block anchor and signs and sends synthetic
// transactions (including the block anchor) for the previously committed block.
func (x *Executor) finalizeBlock(block *Block) error {
	// Load the ledger state
	var ledger *protocol.SystemLedger
	err := block.Batch.Account(x.Describe.Ledger()).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load system ledger: %w", err)
	}

	// Anchor the last non-empty block if it has not been anchored yet. This
	// used to require the non-empty block to be the IMMEDIATELY previous
	// block (ledger.Index == block.Index-1), which assumes the anchor is
	// always recorded on the very next block. Under CometBFT at one block
	// per second that nearly always holds, but DAG-BFT produces a block per
	// committed certificate — dozens per second — and if the one-block
	// window is missed the anchor is never recorded and the anchor sequence
	// stalls (#4054: 4 anchors recorded out of 55 anchored blocks). The new
	// behavior changes when anchors are recorded (which is part of state),
	// so it is version-gated to preserve replay of pre-Kourou history.
	//
	// Note that recording the anchor and dispatching the last block's
	// synthetic messages are INDEPENDENT duties of this function — skipping
	// the anchor must not skip the synthetics.
	if x.globals.Active.ExecutorVersion.V2KourouEnabled() {
		if ledger.Anchor != nil {
			last, err := x.lastAnchoredBlock(block.Batch)
			if err != nil {
				return errors.UnknownError.WithFormat("determine last anchored block: %w", err)
			}
			if ledger.Index > last {
				err = x.recordAnchor(block, ledger)
				if err != nil {
					return errors.UnknownError.WithFormat("send anchor: %w", err)
				}
			}
		}

		// Did anything happen last block?
		if ledger.Index < block.Index-1 {
			return nil
		}
	} else {
		// Did anything happen last block?
		if ledger.Index < block.Index-1 {
			x.logger.Debug("Skipping anchor", "module", "anchoring", "index", ledger.Index)
			return nil
		}

		// Send the anchor first, before synthetic transactions
		err = x.recordAnchor(block, ledger)
		if err != nil {
			return errors.UnknownError.WithFormat("send anchor: %w", err)
		}
	}

	// If the previous block included a directory anchor, send synthetic
	// transactions anchored by that anchor. Use a read-only batch.
	batch := block.Batch.Begin(false)
	defer batch.Discard()
	err = x.sendSyntheticTransactions(batch, ledger, block.IsLeader)
	if err != nil {
		// We didn't write anything so don't break if we get an error. This
		// could be masking a consensus error but I'm too tired to care.
		x.logger.Error("An error occurred while sending synthetic transactions", "error", err, "block", block.Index, "is-leader", block.IsLeader)
	}

	return nil
}

// lastAnchoredBlock returns the block index anchored by the most recently
// recorded anchor, or zero if no anchor has been recorded.
func (x *Executor) lastAnchoredBlock(batch *database.Batch) (uint64, error) {
	sequence := batch.Account(x.Describe.AnchorPool()).AnchorSequenceChain()
	head, err := sequence.Head().Get()
	if err != nil {
		return 0, errors.UnknownError.WithFormat("load anchor sequence chain head: %w", err)
	}
	if head.Count == 0 {
		return 0, nil
	}

	hash, err := sequence.Entry(head.Count - 1)
	if err != nil {
		return 0, errors.UnknownError.WithFormat("load anchor sequence chain entry %d: %w", head.Count-1, err)
	}

	var msg messaging.MessageWithTransaction
	err = batch.Message2(hash).Main().GetAs(&msg)
	if err != nil {
		return 0, errors.UnknownError.WithFormat("load anchor %d: %w", head.Count, err)
	}

	anchor, ok := msg.GetTransaction().Body.(protocol.AnchorBody)
	if !ok {
		return 0, errors.InternalError.WithFormat("anchor sequence entry %d is not an anchor: got %v", head.Count-1, msg.GetTransaction().Body.Type())
	}
	return anchor.GetPartitionAnchor().MinorBlockIndex, nil
}

func (x *Executor) recordAnchor(block *Block, ledger *protocol.SystemLedger) error {
	// Construct the anchor
	anchor, sequenceNumber, err := crosschain.ConstructLastAnchor(block.Context, block.Batch, x.Describe.PartitionUrl().URL)
	if anchor == nil || err != nil {
		return errors.UnknownError.Wrap(err)
	}
	anchorTxn := new(protocol.Transaction)
	anchorTxn.Body = anchor

	// Record the anchor
	err = putMessageWithStatus(block.Batch,
		&messaging.TransactionMessage{Transaction: anchorTxn},
		&protocol.TransactionStatus{Code: errors.Remote})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Add the transaction to the anchor sequence chain
	record := block.Batch.Account(x.Describe.AnchorPool()).AnchorSequenceChain()
	chain, err := record.Get()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	index := chain.Height()
	err = chain.AddEntry(anchorTxn.GetHash(), false)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if index+1 != int64(sequenceNumber) {
		x.logger.Error("Sequence number does not match index chain index", "seq-num", sequenceNumber, "index", index)
	}

	err = block.State.ChainUpdates.DidAddChainEntry(block.Batch, x.Describe.AnchorPool(), record.Name(), record.Type(), anchorTxn.GetHash(), uint64(index), 0, 0)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	if x.Describe.NetworkType == protocol.PartitionTypeDirectory && !x.globals.Active.ExecutorVersion.V2VandenbergEnabled() {
		// As far as I know, the only thing this achieves (besides logging) is
		// ensuring the block is not discarded. The only other reference to
		// OpenedMajorBlock is (*BlockState).Empty. This is not necessary after
		// v2-vandenberg since it uses a synthetic message (which prevents the
		// block from being discarded).
		anchor := anchor.(*protocol.DirectoryAnchor)
		if anchor.MakeMajorBlock > 0 {
			x.logger.Info("Start major block", "major-index", anchor.MakeMajorBlock, "minor-index", ledger.Index)
			block.State.OpenedMajorBlock = true
		}
	}
	return nil
}

func (x *Executor) sendSyntheticTransactions(batch *database.Batch, ledger *protocol.SystemLedger, isLeader bool) error {
	// Check for received anchors
	anchorLedger := batch.Account(x.Describe.AnchorPool())
	anchorIndexLast, anchorIndexPrev, err := indexing.LoadLastTwoIndexEntries(anchorLedger.MainChain().Index())
	if err != nil {
		return errors.InternalError.WithFormat("load last two anchor index chain entries: %w", err)
	}
	if anchorIndexLast == nil {
		return nil // Chain is empty
	}
	to := anchorIndexLast.Source

	if anchorIndexLast.BlockIndex < ledger.Index {
		return nil // Last block did not have an anchor
	}

	var from uint64
	if anchorIndexPrev != nil {
		from = anchorIndexPrev.Source + 1
	}

	anchorChain, err := anchorLedger.MainChain().Get()
	if err != nil {
		return errors.InternalError.WithFormat("load anchor main chain: %w", err)
	}
	entries, err := anchorChain.Entries(int64(from), int64(to+1))
	if err != nil {
		return errors.InternalError.WithFormat("load entries %d to %d of the anchor main chain: %w", from, to, err)
	}

	for i, hash := range entries {
		var msg messaging.MessageWithTransaction
		err := batch.Message2(hash).Main().GetAs(&msg)
		if err != nil {
			return errors.InternalError.WithFormat("load transaction %d of the anchor main chain: %w", from+uint64(i), err)
		}

		// Ignore anything that's not a directory anchor
		anchor, ok := msg.GetTransaction().Body.(*protocol.DirectoryAnchor)
		if !ok {
			continue
		}

		if x.Describe.NetworkType == protocol.PartitionTypeDirectory {
			err = x.sendSyntheticTransactionsForBlock(batch, isLeader, anchor.MinorBlockIndex, nil)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
		}

		for _, receipt := range anchor.Receipts {
			// Ignore receipts for other partitions
			if !x.Describe.PartitionUrl().URL.LocalTo(receipt.Anchor.Source) {
				continue
			}

			err = x.sendSyntheticTransactionsForBlock(batch, isLeader, receipt.Anchor.MinorBlockIndex, receipt)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
		}
	}

	return nil
}

func (x *Executor) sendSyntheticTransactionsForBlock(batch *database.Batch, isLeader bool, blockIndex uint64, blockReceipt *protocol.PartitionAnchorReceipt) error {
	indexIndex, err := batch.SystemData(x.Describe.PartitionId).SyntheticIndexIndex(blockIndex).Get()
	switch {
	case err == nil:
		// Found
	case errors.Is(err, errors.NotFound):
		return nil
	default:
		return errors.InternalError.WithFormat("load synthetic transaction index index for block %d: %w", blockIndex, err)
	}

	// Find the synthetic main chain index entry for the block
	record := batch.Account(x.Describe.Synthetic())
	synthIndexChain, err := record.MainChain().Index().Get()
	if err != nil {
		return errors.InternalError.WithFormat("load synthetic index chain: %w", err)
	}

	indexEntry := new(protocol.IndexEntry)
	err = synthIndexChain.EntryAs(int64(indexIndex), indexEntry)
	if err != nil {
		return errors.InternalError.WithFormat("load synthetic index chain entry %d: %w", indexIndex-1, err)
	}
	to := indexEntry.Source

	// Is there a previous entry?
	var from uint64
	if indexIndex > 0 {
		prevEntry := new(protocol.IndexEntry)
		err = synthIndexChain.EntryAs(int64(indexIndex-1), prevEntry)
		if err != nil {
			return errors.InternalError.WithFormat("load synthetic index chain entry %d: %w", indexIndex-1, err)
		}
		from = prevEntry.Source + 1
	} else {
		from = 1 // Skip genesis
	}

	if blockReceipt == nil {
		x.logger.Debug("Sending synthetic transactions for block", "module", "synthetic", "index", blockIndex)
	} else {
		x.logger.Debug("Sending synthetic transactions for block", "module", "synthetic", "index", blockIndex, "anchor-from", logging.AsHex(blockReceipt.RootChainReceipt.Start).Slice(0, 4), "anchor-to", logging.AsHex(blockReceipt.Anchor).Slice(0, 4))
	}

	// Get the root receipt
	rootReceipt, err := x.getRootReceiptForBlock(batch, indexEntry.Anchor, blockIndex)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Process the transactions
	synthMainChain, err := record.MainChain().Get()
	if err != nil {
		return errors.InternalError.WithFormat("load synthetic main chain: %w", err)
	}

	entries, err := synthMainChain.Entries(int64(from), int64(to+1))
	if err != nil {
		return errors.InternalError.WithFormat("load synthetic main chain entries %d to %d: %w", from, to, err)
	}

	// Load every synthetic message of the block, keeping its absolute position on
	// the synthetic main chain. The position is what a collection proof is built
	// from, and it is also what makes the packages below contiguous.
	outbound := make([]*synthOutbound, 0, len(entries))
	for i, hash := range entries {
		var seq *messaging.SequencedMessage
		err := batch.Message2(hash).Main().GetAs(&seq)
		if err != nil {
			return errors.UnknownError.WithFormat("load synthetic transaction: %w", err)
		}
		if h := seq.Hash(); !bytes.Equal(hash, h[:]) {
			return errors.InternalError.WithFormat("synthetic message stored as %X hashes to %X", hash[:4], h[:4])
		}

		o := &synthOutbound{index: int64(from) + int64(i), seq: seq}

		// Send the transaction along with the signature request/authority
		// signature
		//
		// TODO Make this smarter, only send it the first time?
		if msg, ok := seq.Message.(messaging.MessageForTransaction); ok &&
			seq.Message.Type() != messaging.MessageTypeBlockAnchor {
			var txn messaging.MessageWithTransaction
			err := batch.Message(msg.GetTxID().Hash()).Main().GetAs(&txn)
			if err != nil {
				return errors.UnknownError.WithFormat("load transaction for synthetic message: %w", err)
			}
			o.companion = txn
		}
		outbound = append(outbound, o)
	}

	if !isLeader {
		return nil // Only send synthetic transactions from the leader
	}

	// Group by destination. One package can only share a proof among messages
	// going to the same place, because a package is one envelope.
	byDest := map[string][]*synthOutbound{}
	order := []string{}
	for _, o := range outbound {
		k := o.seq.Destination.String()
		if _, ok := byDest[k]; !ok {
			order = append(order, k)
		}
		byDest[k] = append(byDest[k], o)
	}

	for _, k := range order {
		group := byDest[k]
		// One proof per message is what a single-message package amounts to, and
		// a list of one element is slightly LARGER than the receipt it replaces —
		// so do not pretend. Below the threshold, keep the old form.
		//
		// NOTE: with the receiver-side replica (#4140), even a one-element list
		// pays forward — the destination absorbs it and later messages ride
		// free — so this threshold is now a heuristic, not a hard rule. Left at
		// 2 until the replica's effect is measured.
		if len(group) < synthBundleMin || !x.globals.Active.ExecutorVersion.V2KourouEnabled() {
			for _, o := range group {
				err = x.sendSynthWithOwnProof(batch, o, synthMainChain, rootReceipt, blockReceipt, int64(to))
				if err != nil {
					return errors.UnknownError.Wrap(err)
				}
			}
			continue
		}
		err = x.sendSynthPackages(group, record.MainChain(), rootReceipt, blockReceipt, int64(to))
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	return nil
}

// synthOutbound is one synthetic message awaiting dispatch, with the position on
// the synthetic main chain that a proof is built from.
type synthOutbound struct {
	index     int64
	seq       *messaging.SequencedMessage
	companion messaging.Message // the transaction it refers to, when it has one
}

// synthBundleMin is the smallest group worth bundling. A collection proof over a
// single element carries that element's hash plus an anchoring path, which is no
// smaller than the individual receipt it would replace — so one message keeps the
// old form and pays nothing for machinery it cannot benefit from.
const synthBundleMin = 2

// synthPackageBudget bounds one package's serialized size. An envelope is one
// consensus transaction, and the DAG-BFT worker caps a BATCH at MaxBatchBytes —
// so the budget is DERIVED from that limit, never hard-coded: main's 3 MiB
// constant came from CometBFT's max_tx_bytes and was six times this transport's
// entire batch limit (#4141). A quarter is reserved for what is not known until
// the package is closed: the proof's merkle state and continuation, the
// signatures, and the envelope framing. The per-element hashes are charged
// per-message in the packing loop.
func (x *Executor) synthPackageBudget() int {
	limit := x.MaxEnvelopeSize
	if limit <= 0 {
		limit = dagconfig.DefaultMaxBatchBytes
	}
	return limit - limit/4
}

// sendSynthWithOwnProof dispatches one synthetic message carrying its own
// individual receipt — the pre-#4090 form, kept for single-message groups.
func (x *Executor) sendSynthWithOwnProof(batch *database.Batch, o *synthOutbound, synthMainChain *database.Chain, rootReceipt *merkle.Receipt, blockReceipt *protocol.PartitionAnchorReceipt, to int64) error {
	synthReceipt, err := synthMainChain.Receipt(o.index, to)
	if err != nil {
		return errors.UnknownError.WithFormat("get synthetic main chain receipt from %d to %d: %w", o.index, to, err)
	}

	receipt := new(protocol.AnnotatedReceipt)
	receipt.Anchor = new(protocol.AnchorMetadata)
	receipt.Anchor.Account = protocol.DnUrl()
	if blockReceipt == nil {
		receipt.Receipt, err = synthReceipt.Combine(rootReceipt)
	} else {
		receipt.Receipt, err = synthReceipt.Combine(rootReceipt, blockReceipt.RootChainReceipt)
	}
	if err != nil {
		return errors.UnknownError.WithFormat("combine receipts: %w", err)
	}

	msg, err := x.wrapSynthetic(o.seq, receipt)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	messages := []messaging.Message{msg}
	if o.companion != nil {
		messages = append(messages, o.companion)
	}
	env := &messaging.Envelope{Messages: messages}
	err = x.mainDispatcher.Submit(context.Background(), o.seq.Destination, env)
	if err != nil {
		h := o.seq.Hash()
		return errors.UnknownError.WithFormat("send synthetic transaction %X: %w", h[:4], err)
	}
	return nil
}

// sendSynthPackages dispatches a destination's messages as one or more packages,
// each carrying ONE collection proof covering exactly the messages in it (#4090).
//
// Each package is self-verifying and independent: its proof spans its own
// messages and is continued from there to the same block anchor the individual
// receipts use. Packages may therefore be delivered in any order, and losing one
// does not block another — the property that would be given up by sending the
// proof once and referring back to it from later packages.
func (x *Executor) sendSynthPackages(group []*synthOutbound, synthChain2 *database.Chain2, rootReceipt *merkle.Receipt, blockReceipt *protocol.PartitionAnchorReceipt, to int64) error {
	budget := x.synthPackageBudget()
	for len(group) > 0 {
		// Pack greedily up to the budget, always taking at least one message so
		// an oversized message cannot wedge the loop — it goes alone and the
		// transport rejects it visibly rather than us silently dropping it.
		var pkg []*synthOutbound
		var msgs []messaging.Message
		size := 0
		for len(group) > 0 {
			o := group[0]
			m, err := x.wrapSynthetic(o.seq, nil)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
			add := []messaging.Message{m}
			if o.companion != nil {
				add = append(add, o.companion)
			}
			n := 0
			for _, mm := range add {
				b, err := mm.MarshalBinary()
				if err != nil {
					return errors.UnknownError.WithFormat("marshal synthetic message: %w", err)
				}
				n += len(b)
			}
			// The span grows with every message taken, and the proof carries one
			// hash per element in it, so charge for that too.
			span := int(to-pkg0index(pkg, o)) + 1
			if len(pkg) > 0 && size+n+span*32 > budget {
				break
			}
			size += n
			pkg = append(pkg, o)
			msgs = append(msgs, add...)
			group = group[1:]
		}

		proof, err := x.buildSynthPackageProof(pkg, synthChain2, rootReceipt, blockReceipt, to)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		// The proof leads, so a reader sees it before the messages that need it.
		env := &messaging.Envelope{Messages: append([]messaging.Message{
			&messaging.SyntheticProof{Proof: proof},
		}, msgs...)}
		err = x.mainDispatcher.Submit(context.Background(), pkg[0].seq.Destination, env)
		if err != nil {
			return errors.UnknownError.WithFormat("send synthetic package of %d to %v: %w", len(pkg), pkg[0].seq.Destination, err)
		}
	}
	return nil
}

// pkg0index returns the first index of the package being packed, or o's own
// index when the package is still empty.
func pkg0index(pkg []*synthOutbound, o *synthOutbound) int64 {
	if len(pkg) == 0 {
		return o.index
	}
	return pkg[0].index
}

// buildSynthPackageProof builds the collection proof for one package: a receipt
// list over the package's span, continued to the block's anchor.
//
// The span runs from the first to the last message of the package. It may cover
// elements belonging to OTHER destinations, because the synthetic main chain
// interleaves them — harmless, since extra elements are proven hashes and
// nothing more, and it is what lets the span stay contiguous.
func (x *Executor) buildSynthPackageProof(pkg []*synthOutbound, synthChain2 *database.Chain2, rootReceipt *merkle.Receipt, blockReceipt *protocol.PartitionAnchorReceipt, to int64) (*protocol.AnnotatedReceipt, error) {
	first := pkg[0].index

	// The span runs to the block's LAST synthetic element, not to the package's
	// last message. A receipt list anchors at the end of its span, and the
	// continuation has to start from that anchor — the block anchor is the only
	// point the root receipt is built from, so the list must reach it. Ending the
	// span early leaves an anchor nothing continues from, which Validate rejects
	// outright ("built an invalid receipt list").
	list, err := merkle.GetReceiptList(synthChain2.Inner(), first, to)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("build receipt list %d to %d: %w", first, to, err)
	}

	// Continue to the same place the individual receipts terminate, so the
	// destination's trust check is unchanged.
	cont := rootReceipt
	if blockReceipt != nil {
		cont, err = rootReceipt.Combine(blockReceipt.RootChainReceipt)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("combine receipts: %w", err)
		}
	}
	list.ContinuedReceipt = cont

	if !list.Validate(nil) {
		return nil, errors.InternalError.With("built an invalid receipt list")
	}

	return &protocol.AnnotatedReceipt{
		ReceiptList: list,
		Anchor:      &protocol.AnchorMetadata{Account: protocol.DnUrl()},
	}, nil
}

// wrapSynthetic wraps a sequenced message for dispatch, signed by this node. A
// nil receipt produces a message with no proof of its own, for a package whose
// proof travels separately (#4090).
func (x *Executor) wrapSynthetic(seq *messaging.SequencedMessage, receipt *protocol.AnnotatedReceipt) (messaging.Message, error) {
	h := seq.Hash()
	keySig, err := x.signTransaction(h[:])
	if err != nil {
		return nil, errors.UnknownError.WithFormat("sign message: %w", err)
	}

	if x.globals.Active.ExecutorVersion.V2BaikonurEnabled() {
		return &messaging.SyntheticMessage{Message: seq, Proof: receipt, Signature: keySig}, nil
	}
	return &messaging.BadSyntheticMessage{Message: seq, Proof: receipt, Signature: keySig}, nil
}

func (x *Executor) signTransaction(hash []byte) (protocol.KeySignature, error) {
	if x.Key == nil {
		return nil, errors.InternalError.WithFormat("attempted to sign with a nil key")
	}

	sig, err := new(signing.Builder).
		SetType(protocol.SignatureTypeED25519).
		SetPrivateKey(x.Key).
		SetUrl(protocol.DnUrl().JoinPath(protocol.Network)).
		SetVersion(x.globals.Active.Network.Version).
		SetTimestamp(1).
		Sign(hash)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	ks, ok := sig.(protocol.KeySignature)
	if !ok {
		return nil, errors.InternalError.WithFormat("expected key signature, got %v", sig.Type())
	}

	return ks, nil
}

func (x *Executor) getRootReceiptForBlock(batch *database.Batch, from, block uint64) (*merkle.Receipt, error) {
	// Load the root index chain
	index, err := batch.Account(x.Describe.Ledger()).RootChain().Index().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load root chain: %w", err)
	}
	if index.Height() == 0 {
		return nil, errors.NotFound.With("root index chain is empty")
	}

	// Locate the index entry for the given block
	_, entry, err := indexing.SearchIndexChain(index, uint64(index.Height()-1), indexing.MatchExact, indexing.SearchIndexChainByBlock(block))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("locate block %d root index chain entry: %w", block, err)
	}

	// Load the root chain
	root, err := batch.Account(x.Describe.Ledger()).RootChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load root chain: %w", err)
	}

	// Get a receipt from the entry to the block's anchor
	receipt, err := root.Receipt(int64(from), int64(entry.Source))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get root chain receipt from %d to %d: %w", from, entry.Source, err)
	}
	return receipt, nil
}
