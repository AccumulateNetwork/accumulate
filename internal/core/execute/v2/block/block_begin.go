// Copyright 2025 The Accumulate Authors
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

	// If the previous block included a directory anchor, send synthetic
	// transactions anchored by that anchor
	err = x.sendSyntheticTransactions(block, ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("build synthetic transaction receipts: %w", err)
	}

	return nil
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

func (x *Executor) sendSyntheticTransactions(block *Block, ledger *protocol.SystemLedger) error {
	// Check for received anchors
	anchorLedger := block.Batch.Account(x.Describe.AnchorPool())
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
		err := block.Batch.Message2(hash).Main().GetAs(&msg)
		if err != nil {
			return errors.InternalError.WithFormat("load transaction %d of the anchor main chain: %w", from+uint64(i), err)
		}

		// Ignore anything that's not a directory anchor
		anchor, ok := msg.GetTransaction().Body.(*protocol.DirectoryAnchor)
		if !ok {
			continue
		}

		if x.Describe.NetworkType == protocol.PartitionTypeDirectory {
			err = x.sendSyntheticTransactionsForBlock(block.Batch, block.IsLeader, anchor.MinorBlockIndex, nil)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
		}

		for _, receipt := range anchor.Receipts {
			// Ignore receipts for other partitions
			if !x.Describe.PartitionUrl().URL.LocalTo(receipt.Anchor.Source) {
				continue
			}

			err = x.sendSyntheticTransactionsForBlock(block.Batch, block.IsLeader, receipt.Anchor.MinorBlockIndex, receipt)
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

	// For each synthetic transaction from the last block
	for i, hash := range entries {
		// Load it
		var seq *messaging.SequencedMessage
		err := batch.Message2(hash).Main().GetAs(&seq)
		if err != nil {
			return errors.UnknownError.WithFormat("load synthetic transaction: %w", err)
		}
		if h := seq.Hash(); !bytes.Equal(hash, h[:]) {
			return errors.InternalError.WithFormat("synthetic message stored as %X hashes to %X", hash[:4], h[:4])
		}

		// Get the synthetic main chain receipt
		synthReceipt, err := synthMainChain.Receipt(int64(from)+int64(i), int64(to))
		if err != nil {
			return errors.UnknownError.WithFormat("get synthetic main chain receipt from %d to %d: %w", from, to, err)
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

		h := seq.Hash()
		keySig, err := x.signTransaction(h[:])
		if err != nil {
			return errors.UnknownError.WithFormat("sign message: %w", err)
		}

		messages := []messaging.Message{
			&messaging.BadSyntheticMessage{
				Message:   seq,
				Proof:     receipt,
				Signature: keySig,
			},
		}
		if x.globals.Active.ExecutorVersion.V2BaikonurEnabled() {
			messages = []messaging.Message{
				&messaging.SyntheticMessage{
					Message:   seq,
					Proof:     receipt,
					Signature: keySig,
				},
			}
		}

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
			messages = append(messages, txn)
		}

		// Only send synthetic transactions from the leader
		if isLeader {
			env := &messaging.Envelope{Messages: messages}
			err = x.mainDispatcher.Submit(context.Background(), seq.Destination, env)
			if err != nil {
				return errors.UnknownError.WithFormat("send synthetic transaction %X: %w", hash[:4], err)
			}
		}
	}

	return nil
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
