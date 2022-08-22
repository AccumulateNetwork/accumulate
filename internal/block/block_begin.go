package block

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/shared"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
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

	// Only send anchors from the leader
	if block.IsLeader {
		errs := x.dispatcher.Send(context.Background())
		x.Background(func() {
			for err := range errs {
				switch err := err.(type) {
				case *txnDispatchError:
					x.logger.Error("Failed to dispatch transactions", "error", err.status.Error, "stack", err.status.Error.PrintFullCallstack(), "type", err.typ, "hash", logging.AsHex(err.status.TxID.Hash()).Slice(0, 4))
				default:
					x.logger.Error("Failed to dispatch transactions", "error", fmt.Sprintf("%+v\n", err))
				}
			}
		})
	} else {
		x.dispatcher.Reset()
	}

	// Load the ledger state
	ledger := block.Batch.Account(x.Describe.NodeUrl(protocol.Ledger))
	var ledgerState *protocol.SystemLedger
	err = ledger.GetStateAs(&ledgerState)
	switch {
	case err == nil:
		// Make sure the block index is increasing
		if uint64(ledgerState.Index) >= block.Index {
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
		return errors.Format(errors.StatusUnknownError, "cannot marshal value as json: %w", err)
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
		&protocol.TransactionStatus{Code: errors.StatusDelivered})
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
		return errors.Format(errors.StatusUnknownError, "load system ledger: %w", err)
	}

	// Did anything happen last block?
	if uint64(ledger.Index) < block.Index-1 {
		x.logger.Debug("Skipping anchor", "module", "anchoring", "index", ledger.Index)
		return nil
	}

	// Build receipts for synthetic transactions produced in the previous block
	err = x.sendSyntheticTransactions(block.Batch)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "build synthetic transaction receipts: %w", err)
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
		return errors.Format(errors.StatusUnknownError, "load anchor ledger: %w", err)
	}

	// Send the block anchor
	sequenceNumber := anchorLedger.MinorBlockSequenceNumber
	x.logger.Debug("Anchor block", "module", "anchoring", "index", ledger.Index, "seq-num", sequenceNumber)

	// Load the root chain
	rootChain, err := block.Batch.Account(x.Describe.Ledger()).RootChain().Get()
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load root chain: %w", err)
	}

	stateRoot, err := x.LoadStateRoot(block.Batch)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load state hash: %w", err)
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
		Code:           errors.StatusRemote,
		SourceNetwork:  x.Describe.PartitionUrl().URL,
		SequenceNumber: sequenceNumber,
	})
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	// Add the transaction to the anchor sequence chain
	record := block.Batch.Account(x.Describe.AnchorPool()).AnchorSequenceChain()
	chain, err := record.Get()
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	index := chain.Height()
	err = chain.AddEntry(anchorTxn.GetHash(), false)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}
	if index+1 != int64(sequenceNumber) {
		x.logger.Error("Sequence number does not match index chain index", "seq-num", sequenceNumber, "index", index)
	}

	err = block.State.ChainUpdates.DidAddChainEntry(block.Batch, x.Describe.AnchorPool(), record.Name(), record.Type(), anchorTxn.GetHash(), uint64(index), 0, 0)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	switch x.Describe.NetworkType {
	case config.Directory:
		anchor := anchor.(*protocol.DirectoryAnchor)
		if anchor.MakeMajorBlock > 0 {
			x.logger.Info("Start major block", "major-index", anchor.MajorBlockIndex, "minor-index", ledger.Index)
			block.State.OpenedMajorBlock = true
			x.ExecutorOptions.MajorBlockScheduler.UpdateNextMajorBlockTime(anchor.MakeMajorBlockTime)
		}

		// DN -> BVN
		for _, bvn := range x.Describe.Network.GetBvnNames() {
			err = x.sendBlockAnchor(block.Batch, anchor, sequenceNumber, bvn)
			if err != nil {
				return errors.Format(errors.StatusUnknownError, "send anchor for block %d: %w", ledger.Index, err)
			}
		}

		// DN -> self
		anchor = anchor.Copy() // Make a copy so we don't modify the anchors sent to the BVNs
		anchor.MakeMajorBlock = 0
		err = x.sendBlockAnchor(block.Batch, anchor, sequenceNumber, protocol.Directory)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "send anchor for block %d: %w", ledger.Index, err)
		}

	case config.BlockValidator:
		// BVN -> DN
		err = x.sendBlockAnchor(block.Batch, anchor, sequenceNumber, protocol.Directory)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "send anchor for block %d: %w", ledger.Index, err)
		}
	}

	if x.Describe.NetworkType != config.Directory {
		return nil
	}

	// If we're the DN, send synthetic transactions produced by the block we're
	// anchoring, but send them after the anchor
	err = x.sendSyntheticTransactionsForBlock(block.Batch, ledger.Index, nil)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	return nil
}

func (x *Executor) sendSyntheticTransactions(batch *database.Batch) error {
	// Check for received anchors
	anchorLedger := batch.Account(x.Describe.AnchorPool())
	anchorIndexLast, anchorIndexPrev, err := indexing.LoadLastTwoIndexEntries(anchorLedger.MainChain().Index())
	if err != nil {
		return errors.Format(errors.StatusInternalError, "load last two anchor index chain entries: %w", err)
	}
	if anchorIndexLast == nil {
		return nil // Chain is empty
	}
	to := anchorIndexLast.Source

	systemLedger := batch.Account(x.Describe.Ledger())
	rootIndexPrev, err := indexing.LoadIndexEntryFromEnd(systemLedger.RootChain().Index(), 2)
	if err != nil {
		return errors.Format(errors.StatusInternalError, "load last root index chain entry: %w", err)
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
		return errors.Format(errors.StatusInternalError, "load anchor main chain: %w", err)
	}
	entries, err := anchorChain.Entries(int64(from), int64(to+1))
	if err != nil {
		return errors.Format(errors.StatusInternalError, "load entries %d to %d of the anchor main chain: %w", from, to, err)
	}

	for i, hash := range entries {
		state, err := batch.Transaction(hash).Main().Get()
		if err != nil {
			return errors.Format(errors.StatusInternalError, "load transaction %d of the anchor main chain: %w", from+uint64(i), err)
		}
		if state.Transaction == nil {
			return errors.Format(errors.StatusInternalError, "load transaction %d of the anchor main chain: not a transaction", from+uint64(i))
		}

		// Ignore anything that's not a directory anchor
		anchor, ok := state.Transaction.Body.(*protocol.DirectoryAnchor)
		if !ok {
			continue
		}

		for _, receipt := range anchor.Receipts {
			// Ignore receipts for other partitions
			if !strings.EqualFold(receipt.PartitionID, x.Describe.PartitionId) {
				continue
			}

			err = x.sendSyntheticTransactionsForBlock(batch, receipt.MinorBlockIndex, receipt.RootChainReceipt)
			if err != nil {
				return errors.Wrap(errors.StatusUnknownError, err)
			}
		}
	}

	return nil
}

func (x *Executor) sendSyntheticTransactionsForBlock(batch *database.Batch, blockIndex uint64, blockReceipt *managed.Receipt) error {
	indexIndex, err := batch.SystemData(x.Describe.PartitionId).SyntheticIndexIndex(blockIndex).Get()
	switch {
	case err == nil:
		// Found
	case errors.Is(err, errors.StatusNotFound):
		return nil
	default:
		return errors.Format(errors.StatusInternalError, "load synthetic transaction index index for block %d: %w", blockIndex, err)
	}

	// Find the synthetic main chain index entry for the block
	record := batch.Account(x.Describe.Synthetic())
	synthIndexChain, err := record.MainChain().Index().Get()
	if err != nil {
		return errors.Format(errors.StatusInternalError, "load synthetic index chain: %w", err)
	}

	indexEntry := new(protocol.IndexEntry)
	err = synthIndexChain.EntryAs(int64(indexIndex), indexEntry)
	if err != nil {
		return errors.Format(errors.StatusInternalError, "load synthetic index chain entry %d: %w", indexIndex-1, err)
	}
	to := indexEntry.Source

	// Is there a previous entry?
	var from uint64
	if indexIndex > 0 {
		prevEntry := new(protocol.IndexEntry)
		err = synthIndexChain.EntryAs(int64(indexIndex-1), prevEntry)
		if err != nil {
			return errors.Format(errors.StatusInternalError, "load synthetic index chain entry %d: %w", indexIndex-1, err)
		}
		from = prevEntry.Source + 1
	}

	if blockReceipt == nil {
		x.logger.Debug("Sending synthetic transactions for block", "module", "synthetic", "index", blockIndex)
	} else {
		x.logger.Debug("Sending synthetic transactions for block", "module", "synthetic", "index", blockIndex, "anchor-from", logging.AsHex(blockReceipt.Start).Slice(0, 4), "anchor-to", logging.AsHex(blockReceipt.Anchor).Slice(0, 4))
	}

	// Process the transactions
	synthMainChain, err := record.MainChain().Get()
	if err != nil {
		return errors.Format(errors.StatusInternalError, "load synthetic main chain: %w", err)
	}

	entries, err := synthMainChain.Entries(int64(from), int64(to+1))
	if err != nil {
		return errors.Format(errors.StatusInternalError, "load synthetic main chain entries %d to %d: %w", from, to, err)
	}

	// For each synthetic transaction from the last block
	for _, hash := range entries {
		// Load it
		record := batch.Transaction(hash)
		state, err := record.GetState()
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "load synthetic transaction: %w", err)
		}
		txn := state.Transaction
		if txn.Body.Type() == protocol.TransactionTypeSystemGenesis {
			continue // Genesis is added to partition/synthetic#chain/main, but it's not a real synthetic transaction
		}

		if !bytes.Equal(hash, txn.GetHash()) {
			return errors.Format(errors.StatusInternalError, "%v stored as %X hashes to %X", txn.Body.Type(), hash[:4], txn.GetHash()[:4])
		}

		status, err := record.GetStatus()
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "load synthetic transaction status: %w", err)
		}
		if status.DestinationNetwork == nil {
			return errors.Format(errors.StatusInternalError, "synthetic transaction destination is not set")
		}

		var signatures []protocol.Signature

		partSig := new(protocol.PartitionSignature)
		partSig.SourceNetwork = status.SourceNetwork
		partSig.DestinationNetwork = status.DestinationNetwork
		partSig.SequenceNumber = status.SequenceNumber
		partSig.TransactionHash = *(*[32]byte)(txn.GetHash())
		signatures = append(signatures, partSig)

		localReceipt := new(protocol.ReceiptSignature)
		localReceipt.SourceNetwork = status.SourceNetwork
		localReceipt.Proof = *status.Proof
		localReceipt.TransactionHash = *(*[32]byte)(txn.GetHash())
		signatures = append(signatures, localReceipt)

		if blockReceipt != nil {
			dnReceipt := new(protocol.ReceiptSignature)
			dnReceipt.SourceNetwork = protocol.DnUrl()
			dnReceipt.Proof = *blockReceipt
			dnReceipt.TransactionHash = *(*[32]byte)(txn.GetHash())
			signatures = append(signatures, dnReceipt)
		}

		keySig, err := shared.SignTransaction(&x.Describe, x.Key, batch, txn, status.DestinationNetwork)
		if err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}
		signatures = append(signatures, keySig)

		env := &protocol.Envelope{Transaction: []*protocol.Transaction{txn}, Signatures: signatures}
		err = x.dispatcher.BroadcastTx(context.Background(), txn.Header.Principal, env)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "send synthetic transaction %X: %w", hash[:4], err)
		}
	}

	return nil
}

func (x *Executor) sendBlockAnchor(batch *database.Batch, anchor protocol.AnchorBody, sequenceNumber uint64, destPart string) error {
	destPartUrl := protocol.PartitionUrl(destPart)
	env, err := shared.PrepareBlockAnchor(&x.Describe, x.Key, batch, anchor, sequenceNumber, destPartUrl)
	if err != nil {
		return errors.Wrap(errors.StatusInternalError, err)
	}

	// Send
	err = x.dispatcher.BroadcastTx(context.Background(), destPartUrl, env)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	return nil
}
