// Copyright 2023 The Accumulate Authors
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
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/shared"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// BeginBlock implements ./Chain
func (x *Executor) BeginBlock(block *Block) error {
	//clear the timers
	x.BlockTimers.Reset()

	r := x.BlockTimers.Start(BlockTimerTypeBeginBlock)
	defer x.BlockTimers.Stop(r)

	x.logger.Debug("Begin block", "module", "block", "height", block.Index, "leader", block.IsLeader, "time", block.Time)

	// Finalize the previous block
	err := x.finalizeBlock(block)
	if err != nil {
		return err
	}

	errs := x.mainDispatcher.Send(context.Background())
	x.BackgroundTaskLauncher(func() {
		for err := range errs {
			switch err := err.(type) {
			case protocol.TransactionStatusError:
				x.logger.Error("Failed to dispatch transactions", "error", err, "stack", err.TransactionStatus.Error.PrintFullCallstack(), "txid", err.TxID)
			default:
				x.logger.Error("Failed to dispatch transactions", "error", err, "stack", fmt.Sprintf("%+v\n", err))
			}
		}
	})

	// Load the ledger state
	ledger := block.Batch.Account(x.Describe.NodeUrl(protocol.Ledger))
	var ledgerState *protocol.SystemLedger
	err = ledger.GetStateAs(&ledgerState)
	switch {
	case err == nil:
		// Make sure the block index is increasing
		if ledgerState.Index >= block.Index {
			panic(fmt.Errorf("current height is %d but the next block height is %d", ledgerState.Index, block.Index))
		}

	case x.isGenesis && errors.Is(err, storage.ErrNotFound):
		// OK

	default:
		return fmt.Errorf("cannot load ledger: %w", err)
	}
	lastBlockWasEmpty := ledgerState.Index < block.Index-1

	// Reset transient values
	ledgerState.Index = block.Index
	ledgerState.Timestamp = block.Time
	ledgerState.PendingUpdates = nil
	ledgerState.AcmeBurnt = *big.NewInt(0)
	ledgerState.Anchor = nil

	err = ledger.PutState(ledgerState)
	if err != nil {
		return fmt.Errorf("cannot write ledger: %w", err)
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

	return nil
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
	wd.Entry = &protocol.AccumulateDataEntry{Data: [][]byte{data}}
	dataAccountUrl := x.Describe.NodeUrl(internalAccountPath)

	var signer protocol.Signer
	signerUrl := x.Describe.OperatorsPage()
	err = batch.Account(signerUrl).GetStateAs(&signer)
	if err != nil {
		return err
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = x.Describe.NodeUrl()
	txn.Body = &wd
	txn.Header.Initiator = signerUrl.AccountID32()

	st := chain.NewStateManager(&x.Describe, &x.globals.Active, batch.Begin(true), nil, txn, x.logger)
	defer st.Discard()

	var da *protocol.DataAccount
	va := batch.Account(dataAccountUrl)
	err = va.GetStateAs(&da)
	if err != nil {
		return err
	}

	st.UpdateData(da, wd.Entry.Hash(), wd.Entry)

	err = putSyntheticTransaction(
		batch, txn,
		&protocol.TransactionStatus{Code: errors.Delivered})
	if err != nil {
		return err
	}

	_, err = st.Commit()
	return err
}

// finalizeBlock builds the block anchor and signs and sends synthetic
// transactions (including the block anchor) for the previously committed block.
func (x *Executor) finalizeBlock(block *Block) error {
	// Load the ledger state
	var ledger *protocol.SystemLedger
	err := block.Batch.Account(x.Describe.Ledger()).GetStateAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load system ledger: %w", err)
	}

	// Did anything happen last block?
	if ledger.Index < block.Index-1 {
		x.logger.Debug("Skipping anchor", "module", "anchoring", "index", ledger.Index)
		return nil
	}

	// Build receipts for synthetic transactions produced in the previous block
	err = x.sendSyntheticTransactions(block.Batch, block.IsLeader)
	if err != nil {
		return errors.UnknownError.WithFormat("build synthetic transaction receipts: %w", err)
	}

	// Is there an anchor to send?
	if ledger.Anchor == nil {
		x.logger.Debug("Skipping anchor", "module", "anchoring", "index", ledger.Index)
		return nil
	}

	// Load the anchor ledger state
	var anchorLedger *protocol.AnchorLedger
	err = block.Batch.Account(x.Describe.AnchorPool()).GetStateAs(&anchorLedger)
	if err != nil {
		return errors.UnknownError.WithFormat("load anchor ledger: %w", err)
	}

	// Send the block anchor
	sequenceNumber := anchorLedger.MinorBlockSequenceNumber
	x.logger.Debug("Anchor block", "module", "anchoring", "index", ledger.Index, "seq-num", sequenceNumber)

	// Load the root chain
	rootChain, err := block.Batch.Account(x.Describe.Ledger()).RootChain().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load root chain: %w", err)
	}

	stateRoot, err := x.LoadStateRoot(block.Batch)
	if err != nil {
		return errors.UnknownError.WithFormat("load state hash: %w", err)
	}

	anchor := ledger.Anchor.CopyAsInterface().(protocol.AnchorBody)
	partAnchor := anchor.GetPartitionAnchor()
	partAnchor.RootChainIndex = uint64(rootChain.Height()) - 1
	partAnchor.RootChainAnchor = *(*[32]byte)(rootChain.Anchor())
	partAnchor.StateTreeAnchor = *(*[32]byte)(stateRoot)
	anchorTxn := new(protocol.Transaction)
	anchorTxn.Body = anchor

	// Record the anchor
	err = putSyntheticTransaction(block.Batch, anchorTxn, &protocol.TransactionStatus{
		Code: errors.Remote,
	})
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

	switch x.Describe.NetworkType {
	case config.Directory:
		anchor := anchor.(*protocol.DirectoryAnchor)
		if anchor.MakeMajorBlock > 0 {
			x.logger.Info("Start major block", "major-index", anchor.MakeMajorBlock, "minor-index", ledger.Index)
			block.State.OpenedMajorBlock = true
			x.ExecutorOptions.MajorBlockScheduler.UpdateNextMajorBlockTime(anchor.MakeMajorBlockTime)
		}

		// DN -> BVN
		for _, bvn := range x.Describe.Network.GetBvnNames() {
			err = x.sendBlockAnchor(block.Batch, anchor, sequenceNumber, bvn)
			if err != nil {
				return errors.UnknownError.WithFormat("send anchor for block %d: %w", ledger.Index, err)
			}
		}

		// DN -> self
		anchor = anchor.Copy() // Make a copy so we don't modify the anchors sent to the BVNs
		anchor.MakeMajorBlock = 0
		err = x.sendBlockAnchor(block.Batch, anchor, sequenceNumber, protocol.Directory)
		if err != nil {
			return errors.UnknownError.WithFormat("send anchor for block %d: %w", ledger.Index, err)
		}

	case config.BlockValidator:
		// BVN -> DN
		err = x.sendBlockAnchor(block.Batch, anchor, sequenceNumber, protocol.Directory)
		if err != nil {
			return errors.UnknownError.WithFormat("send anchor for block %d: %w", ledger.Index, err)
		}
	}

	if x.Describe.NetworkType != config.Directory {
		return nil
	}

	// If we're the DN, send synthetic transactions produced by the block we're
	// anchoring, but send them after the anchor
	err = x.sendSyntheticTransactionsForBlock(block.Batch, block.IsLeader, ledger.Index, nil)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return nil
}

func (x *Executor) sendSyntheticTransactions(batch *database.Batch, isLeader bool) error {
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

	systemLedger := batch.Account(x.Describe.Ledger())
	rootIndexPrev, err := indexing.LoadIndexEntryFromEnd(systemLedger.RootChain().Index(), 2)
	if err != nil {
		return errors.InternalError.WithFormat("load last root index chain entry: %w", err)
	}

	if rootIndexPrev != nil && anchorIndexLast.Source >= rootIndexPrev.Source {
		return nil // Entries are from last block
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
		state, err := batch.Transaction(hash).Main().Get()
		if err != nil {
			return errors.InternalError.WithFormat("load transaction %d of the anchor main chain: %w", from+uint64(i), err)
		}
		if state.Transaction == nil {
			return errors.InternalError.WithFormat("load transaction %d of the anchor main chain: not a transaction", from+uint64(i))
		}

		// Ignore anything that's not a directory anchor
		anchor, ok := state.Transaction.Body.(*protocol.DirectoryAnchor)
		if !ok {
			continue
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
	}

	if blockReceipt == nil {
		x.logger.Debug("Sending synthetic transactions for block", "module", "synthetic", "index", blockIndex)
	} else {
		x.logger.Debug("Sending synthetic transactions for block", "module", "synthetic", "index", blockIndex, "anchor-from", logging.AsHex(blockReceipt.RootChainReceipt.Start).Slice(0, 4), "anchor-to", logging.AsHex(blockReceipt.Anchor).Slice(0, 4))
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
	for _, hash := range entries {
		// Load it
		record := batch.Transaction(hash)
		state, err := record.GetState()
		if err != nil {
			return errors.UnknownError.WithFormat("load synthetic transaction: %w", err)
		}
		txn := state.Transaction
		if txn.Body.Type() == protocol.TransactionTypeSystemGenesis {
			continue // Genesis is added to partition/synthetic#chain/main, but it's not a real synthetic transaction
		}

		if !bytes.Equal(hash, txn.GetHash()) {
			return errors.InternalError.WithFormat("%v stored as %X hashes to %X", txn.Body.Type(), hash[:4], txn.GetHash()[:4])
		}

		status, err := record.GetStatus()
		if err != nil {
			return errors.UnknownError.WithFormat("load synthetic transaction status: %w", err)
		}

		receipt := new(protocol.AnnotatedReceipt)
		receipt.Anchor = new(protocol.AnchorMetadata)
		if status.GotDirectoryReceipt {
			receipt.Receipt = status.Proof
			receipt.Anchor.Account = protocol.DnUrl()
		} else if blockReceipt == nil {
			receipt.Anchor.Account = txn.Header.Source
			receipt.Receipt = status.Proof
		} else {
			receipt.Anchor.Account = protocol.DnUrl()
			receipt.Receipt, err = status.Proof.Combine(blockReceipt.RootChainReceipt)
			if err != nil {
				return errors.UnknownError.WithFormat("combine receipts: %w", err)
			}
		}

		keySig, err := shared.SignTransaction(x.globals.Active.Network, x.Key, batch, txn, txn.Header.Source)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		messages := []messaging.Message{
			&messaging.SyntheticTransaction{
				Transaction: txn,
				Proof:       receipt,
			},
			&messaging.UserSignature{
				Signature:       keySig,
				TransactionHash: txn.ID().Hash(),
			},
		}

		// Only send synthetic transactions from the leader
		if isLeader {
			env := &messaging.Envelope{Messages: messages}
			err = x.mainDispatcher.Submit(context.Background(), txn.Header.Principal, env)
			if err != nil {
				return errors.UnknownError.WithFormat("send synthetic transaction %X: %w", hash[:4], err)
			}
		}
	}

	return nil
}

func (x *Executor) sendBlockAnchor(batch *database.Batch, anchor protocol.AnchorBody, sequenceNumber uint64, destPart string) error {
	// Only send anchors from a validator
	if !x.isValidator {
		return nil
	}

	// If we're on the DN,  the last block updated to v2, and the destination is
	// a BVN, then we must send out the anchor as a v1 anchor since the BVNs
	// will still be running v1
	destPartUrl := protocol.PartitionUrl(destPart)
	if x.Describe.NetworkType == config.Directory && didUpdateToV2(anchor) && !strings.EqualFold(destPart, protocol.Directory) {
		env, err := shared.PrepareBlockAnchor(&x.Describe, x.globals.Active.Network, x.Key, batch, anchor, sequenceNumber, destPartUrl)
		if err != nil {
			return errors.InternalError.Wrap(err)
		}

		err = x.mainDispatcher.Submit(context.Background(), destPartUrl, env)
		return errors.UnknownError.Wrap(err)
	}

	// Create the transaction
	txn := new(protocol.Transaction)
	txn.Header.Principal = destPartUrl.JoinPath(protocol.AnchorPool)
	txn.Header.Source = x.Describe.NodeUrl()
	txn.Header.Destination = destPartUrl
	txn.Header.SequenceNumber = sequenceNumber
	txn.Body = anchor

	// Create a key signature
	keySig, err := shared.SignTransaction(x.globals.Active.Network, x.Key, batch, txn, destPartUrl)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Dispatch the envelope
	env := &messaging.Envelope{Transaction: []*protocol.Transaction{txn}, Signatures: []protocol.Signature{keySig}}
	err = x.mainDispatcher.Submit(context.Background(), destPartUrl, env)
	return errors.UnknownError.Wrap(err)
}

func didUpdateToV2(anchor protocol.AnchorBody) bool {
	// TODO Is there a better way to check for a recent version change?

	dir, ok := anchor.(*protocol.DirectoryAnchor)
	if !ok {
		return false
	}

	for _, update := range dir.Updates {
		update, ok := update.Body.(*protocol.ActivateProtocolVersion)
		if ok && update.Version.V2() {
			return true
		}
	}
	return false
}
