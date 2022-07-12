package block

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	jrpc "github.com/tendermint/tendermint/rpc/jsonrpc/types"
	tm "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// BeginBlock implements ./Chain
func (x *Executor) BeginBlock(block *Block) error {
	//clear the timers
	x.BlockTimers.Reset()

	r := x.BlockTimers.Start(BlockTimerTypeBeginBlock)
	defer x.BlockTimers.Stop(r)

	x.logger.Debug("Begin block", "height", block.Index, "leader", block.IsLeader, "time", block.Time)

	// Finalize the previous block
	err := x.finalizeBlock(block)
	if err != nil {
		return err
	}

	// Reset the block state
	err = indexing.BlockState(block.Batch, x.Describe.NodeUrl(protocol.Ledger)).Clear()
	if err != nil {
		return err
	}

	// Load the ledger state
	ledger := block.Batch.Account(x.Describe.NodeUrl(protocol.Ledger))
	var ledgerState *protocol.SystemLedger
	err = ledger.GetStateAs(&ledgerState)
	switch {
	case err == nil:
		// Make sure the block index is increasing
		if uint64(ledgerState.Index) >= block.Index {
			panic(fmt.Errorf("Current height is %d but the next block height is %d!", ledgerState.Index, block.Index))
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

	// Nothing to do
	if uint64(ledger.Index) < block.Index-1 || ledger.Anchor == nil {
		x.logger.Debug("Skipping anchor", "index", ledger.Index)
		return nil
	}

	// Build receipts for synthetic transactions produced in the previous block
	err = x.sendSyntheticTransactions(block)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "build synthetic transaction receipts: %w", err)
	}

	// Load the anchor ledger state
	var anchorLedger *protocol.AnchorLedger
	err = block.Batch.Account(x.Describe.AnchorPool()).GetStateAs(&anchorLedger)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load anchor ledger: %w", err)
	}

	// Send the block anchor
	sequenceNumber := anchorLedger.MinorBlockSequenceNumber
	x.logger.Debug("Anchor block", "index", ledger.Index, "seq-num", sequenceNumber)

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
			x.logger.Info("Start major block", "major-index", anchor.MajorBlockIndex, "minor-index", block.Index)
			block.State.OpenedMajorBlock = true
			x.ExecutorOptions.MajorBlockScheduler.UpdateNextMajorBlockTime(anchor.MakeMajorBlockTime)
		}

		// DN -> BVN
		for _, bvn := range x.Describe.Network.GetBvnNames() {
			err = x.sendBlockAnchor(block.Batch, anchor, sequenceNumber, bvn)
			if err != nil {
				return errors.Format(errors.StatusUnknownError, "send anchor for block %d: %w", block.Index, err)
			}
		}

		// DN -> self
		anchor = anchor.Copy() // Make a copy so we don't modify the anchors sent to the BVNs
		anchor.MakeMajorBlock = 0
		err = x.sendBlockAnchor(block.Batch, anchor, sequenceNumber, protocol.Directory)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "send anchor for block %d: %w", block.Index, err)
		}

	case config.BlockValidator:
		// BVN -> DN
		err = x.sendBlockAnchor(block.Batch, anchor, sequenceNumber, protocol.Directory)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "send anchor for block %d: %w", block.Index, err)
		}
	}

	// Only send anchors from the leader
	if !block.IsLeader {
		x.dispatcher.Reset()
		return nil
	}

	errs := x.dispatcher.Send(context.Background())
	x.Background(func() {
		for err := range errs {
			x.checkDispatchError(err, func(err error) {
				switch err := err.(type) {
				case *txnDispatchError:
					x.logger.Error("Failed to dispatch transactions", "error", err.status.Error, "stack", err.status.Error.PrintFullCallstack(), "type", err.typ, "hash", logging.AsHex(err.status.TxID.Hash()).Slice(0, 4))
				default:
					x.logger.Error("Failed to dispatch transactions", "error", fmt.Sprintf("%+v\n", err))
				}
			})
		}
	})

	return nil
}

func (x *Executor) sendSyntheticTransactions(block *Block) error {
	// Load the root chain's index chain's last two entries
	last, nextLast, err := indexing.LoadLastTwoIndexEntries(block.Batch.Account(x.Describe.Ledger()).RootChain().Index())
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load root index chain's last two entries: %w", err)
	}
	if last == nil {
		return nil // Root chain is empty
	}
	var prevRootAnchor uint64
	if nextLast != nil {
		prevRootAnchor = nextLast.Source + 1
	}

	// Load the synthetic transaction chain's index chain's last two entries
	last, nextLast, err = indexing.LoadLastTwoIndexEntries(block.Batch.Account(x.Describe.Synthetic()).MainChain().Index())
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load synthetic transaction index chain's last two entries: %w", err)
	}
	if last == nil {
		return nil // Synth chain is empty
	}
	synthEnd, synthAnchor := last.Source, last.Anchor
	var synthStart uint64
	if nextLast != nil {
		synthStart = nextLast.Source + 1
	}
	if synthAnchor < prevRootAnchor {
		return nil // No change since last block
	}

	// Load the synthetic transaction chain
	chain, err := block.Batch.Account(x.Describe.Synthetic()).MainChain().Get()
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load root chain: %w", err)
	}

	// Get transaction hashes
	hashes, err := chain.Entries(int64(synthStart), int64(synthEnd)+1)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load entries %d through %d of synthetic transaction chain: %w", synthStart, synthEnd, err)
	}

	// For each synthetic transaction from the last block
	for _, hash := range hashes {
		// Load it
		record := block.Batch.Transaction(hash)
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

		partSig := new(protocol.PartitionSignature)
		partSig.SourceNetwork = status.SourceNetwork
		partSig.DestinationNetwork = status.DestinationNetwork
		partSig.SequenceNumber = status.SequenceNumber
		partSig.TransactionHash = *(*[32]byte)(txn.GetHash())

		receiptSig := new(protocol.ReceiptSignature)
		receiptSig.SourceNetwork = status.SourceNetwork
		receiptSig.Proof = *status.Proof
		receiptSig.TransactionHash = *(*[32]byte)(txn.GetHash())

		keySig, err := x.signTransaction(block.Batch, txn, status.DestinationNetwork)
		if err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}

		env := &protocol.Envelope{Transaction: []*protocol.Transaction{txn}, Signatures: []protocol.Signature{partSig, receiptSig, keySig}}
		err = x.dispatcher.BroadcastTx(context.Background(), txn.Header.Principal, env)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "send synthetic transaction %X: %w", hash[:4], err)
		}
	}

	return nil
}

func (x *Executor) signTransaction(batch *database.Batch, txn *protocol.Transaction, destination *url.URL) (protocol.Signature, error) {
	var page *protocol.KeyPage
	err := batch.Account(x.Describe.OperatorsPage()).GetStateAs(&page)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load operator key page: %w", err)
	}

	// Sign it
	bld := new(signing.Builder).
		SetType(protocol.SignatureTypeED25519).
		SetPrivateKey(x.Key).
		SetUrl(config.NetworkUrl{URL: destination}.OperatorsPage()).
		SetVersion(1).
		SetTimestamp(1)

	keySig, err := bld.Sign(txn.GetHash())
	if err != nil {
		return nil, errors.Format(errors.StatusInternalError, "sign synthetic transaction: %w", err)
	}

	return keySig, nil
}

func (x *Executor) prepareBlockAnchor(batch *database.Batch, anchor protocol.TransactionBody, sequenceNumber uint64, destPartUrl *url.URL) (*protocol.Envelope, error) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = destPartUrl.JoinPath(protocol.AnchorPool)
	txn.Body = anchor

	// Create a synthetic origin signature
	initSig, err := new(signing.Builder).
		SetUrl(x.Describe.NodeUrl()).
		SetVersion(sequenceNumber).
		InitiateSynthetic(txn, destPartUrl)
	if err != nil {
		return nil, errors.Wrap(errors.StatusInternalError, err)
	}

	// Create a key signature
	keySig, err := x.signTransaction(batch, txn, initSig.DestinationNetwork)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	return &protocol.Envelope{Transaction: []*protocol.Transaction{txn}, Signatures: []protocol.Signature{initSig, keySig}}, nil
}

func (x *Executor) sendBlockAnchor(batch *database.Batch, anchor protocol.AnchorBody, sequenceNumber uint64, destPart string) error {
	destPartUrl := protocol.PartitionUrl(destPart)
	env, err := x.prepareBlockAnchor(batch, anchor, sequenceNumber, destPartUrl)
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

// checkDispatchError returns nil if the error can be ignored.
func (x *Executor) checkDispatchError(err error, fn func(error)) {
	if err == nil {
		return
	}

	// TODO This may be unnecessary once this issue is fixed:
	// https://github.com/tendermint/tendermint/issues/7185.

	// Is the error "tx already exists in cache"?
	if err.Error() == tm.ErrTxInCache.Error() {
		return
	}

	// Or RPC error "tx already exists in cache"?
	var rpcErr1 *jrpc.RPCError
	if errors.As(err, &rpcErr1) && *rpcErr1 == *errTxInCache1 {
		return
	}

	var rpcErr2 jsonrpc2.Error
	if errors.As(err, &rpcErr2) && rpcErr2 == errTxInCache2 {
		return
	}

	var errorsErr *errors.Error
	if errors.As(err, &errorsErr) {
		// This probably should not be necessary
		if errorsErr.Code == errors.StatusDelivered {
			return
		}
	}

	// It's a real error
	fn(err)
}
