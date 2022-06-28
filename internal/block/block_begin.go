package block

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	jrpc "github.com/tendermint/tendermint/rpc/jsonrpc/types"
	tm "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
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
	ledgerState.SendAnchor = false
	ledgerState.OpenMajorBlock = false
	ledgerState.ClosedMajorBlock = 0

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
		return errors.Format(errors.StatusUnknownError, "load ledger: %w", err)
	}

	var anchor *protocol.AnchorLedger
	record := block.Batch.Account(x.Describe.AnchorPool())
	err = record.GetStateAs(&anchor)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load anchor ledger: %w", err)
	}

	var openMajor uint64
	var majorBlockTime time.Time
	if ledger.OpenMajorBlock {
		x.logger.Info("Start major block", "major-index", anchor.MajorBlockIndex, "minor-index", block.Index)
		openMajor, majorBlockTime = anchor.MajorBlockIndex, anchor.MajorBlockTime
		block.State.OpenedMajorBlock = true
		x.ExecutorOptions.MajorBlockScheduler.UpdateNextMajorBlockTime(block.Time)
	}

	majorBlockIndex := ledger.ClosedMajorBlock

	// If the last block is more than one block old, it has already been finalized
	if uint64(ledger.Index) < block.Index-1 && openMajor == 0 && majorBlockIndex == 0 {
		return nil
	}

	// Build receipts for synthetic transactions produced in the previous block
	err = x.sendSyntheticTransactions(block)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "build synthetic transaction receipts: %w", err)
	}

	// Don't send an anchor if nothing happened beyond receiving an anchor
	if !ledger.SendAnchor {
		x.logger.Debug("Skipping anchor", "index", ledger.Index)
		return nil
	}

	// Send the block anchor
	sequenceNumber := anchor.MinorBlockSequenceNumber
	x.logger.Debug("Anchor block", "index", ledger.Index, "seq-num", sequenceNumber)

	switch x.Describe.NetworkType {
	case config.Directory:
		// DN -> all BVNs
		anchor, err := x.buildDirectoryAnchor(block.Batch, ledger, openMajor, majorBlockIndex, majorBlockTime)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "build block anchor: %w", err)
		}

		// Only send anchors from the leader
		if !block.IsLeader {
			break
		}

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
		anchor, err := x.buildPartitionAnchor(block.Batch, ledger, majorBlockIndex)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "build block anchor: %w", err)
		}

		// Only send anchors from the leader
		if !block.IsLeader {
			break
		}

		err = x.sendBlockAnchor(block.Batch, anchor, sequenceNumber, protocol.Directory)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "send anchor for block %d: %w", block.Index, err)
		}
	}

	errs := x.dispatcher.Send(context.Background())
	go func() {
		for err := range errs {
			x.checkDispatchError(err, func(err error) {
				x.logger.Error("Failed to dispatch transactions", "error", fmt.Sprintf("%+v\n", err))
			})
		}
	}()

	return nil
}

func (x *Executor) sendSyntheticTransactions(block *Block) error {
	// Load the root chain's index chain's last two entries
	last, nextLast, err := indexing.LoadLastTwoIndexEntries(block.Batch.Account(x.Describe.Ledger()), protocol.MinorRootIndexChain)
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
	last, nextLast, err = indexing.LoadLastTwoIndexEntries(block.Batch.Account(x.Describe.Synthetic()), protocol.IndexChain(protocol.MainChain, false))
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
	chain, err := block.Batch.Account(x.Describe.Synthetic()).ReadChain(protocol.MainChain)
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

		// Only send synthetic transactions from the leader
		if !block.IsLeader {
			continue
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

func (x *Executor) buildDirectoryAnchor(batch *database.Batch, ledgerState *protocol.SystemLedger, openMajor,
	majorBlockIndex uint64, majorBlockTime time.Time) (*protocol.DirectoryAnchor, error) {

	anchor := new(protocol.DirectoryAnchor)
	ledger := batch.Account(x.Describe.Ledger())
	rootChain, err := x.buildBlockAnchor(batch, ledgerState, ledger, &anchor.PartitionAnchor, majorBlockIndex)
	if err != nil {
		return nil, err
	}
	anchor.MakeMajorBlock = openMajor
	anchor.MakeMajorBlockTime = majorBlockTime
	anchor.Updates = ledgerState.PendingUpdates

	// TODO This is pretty inefficient; we're constructing a receipt for every
	// anchor. If we were more intelligent about it, we could send just the
	// Merkle state and a list of transactions, though we would need that for
	// the root chain and each anchor chain.

	anchorUrl := x.Describe.NodeUrl(protocol.AnchorPool)
	record := batch.Account(anchorUrl)

	// Load the list of chain updates
	updates, err := indexing.BlockChainUpdates(batch, &x.Describe, uint64(ledgerState.Index)).Get()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load block chain updates index: %w", err)
	}

	for _, update := range updates.Entries {
		// Is it an anchor chain?
		if update.Type != protocol.ChainTypeAnchor {
			continue
		}

		// Does it belong to our anchor pool?
		if !update.Account.Equal(anchorUrl) {
			continue
		}

		indexChain, err := record.ReadIndexChain(update.Name, false)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "load minor index chain of intermediate anchor chain %s: %w", update.Name, err)
		}

		from, to, anchorIndex, err := getRangeFromIndexEntry(indexChain, uint64(indexChain.Height())-1)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "load range from minor index chain of intermediate anchor chain %s: %w", update.Name, err)
		}

		rootReceipt, err := rootChain.Receipt(int64(anchorIndex), rootChain.Height()-1)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "build receipt for the root chain: %w", err)
		}

		anchorChain, err := record.ReadChain(update.Name)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "load intermediate anchor chain %s: %w", update.Name, err)
		}

		for i := from; i <= to; i++ {
			anchorReceipt, err := anchorChain.Receipt(int64(i), int64(to))
			if err != nil {
				return nil, errors.Format(errors.StatusUnknownError, "build receipt for intermediate anchor chain %s: %w", update.Name, err)
			}

			receipt, err := anchorReceipt.Combine(rootReceipt)
			if err != nil {
				return nil, errors.Format(errors.StatusUnknownError, "build receipt for intermediate anchor chain %s: %w", update.Name, err)
			}

			anchor.Receipts = append(anchor.Receipts, *receipt)
		}
	}

	return anchor, nil
}

func (x *Executor) buildPartitionAnchor(batch *database.Batch, ledgerState *protocol.SystemLedger, majorBlockIndex uint64) (*protocol.BlockValidatorAnchor, error) {
	anchor := new(protocol.BlockValidatorAnchor)
	ledger := batch.Account(x.Describe.Ledger())
	_, err := x.buildBlockAnchor(batch, ledgerState, ledger, &anchor.PartitionAnchor, majorBlockIndex)
	if err != nil {
		return nil, err
	}
	anchor.AcmeBurnt = ledgerState.AcmeBurnt
	return anchor, nil
}

func (x *Executor) buildBlockAnchor(batch *database.Batch, ledgerState *protocol.SystemLedger, ledger *database.Account, anchor *protocol.PartitionAnchor, majorBlockIndex uint64) (*database.Chain, error) {
	// Load the root chain
	rootChain, err := ledger.ReadChain(protocol.MinorRootChain)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load root chain: %w", err)
	}

	stateRoot, err := x.LoadStateRoot(batch)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load state hash: %w", err)
	}

	anchor.Source = x.Describe.NodeUrl()
	anchor.RootChainIndex = uint64(rootChain.Height()) - 1
	anchor.RootChainAnchor = *(*[32]byte)(rootChain.Anchor())
	anchor.StateTreeAnchor = *(*[32]byte)(stateRoot)
	anchor.MinorBlockIndex = uint64(ledgerState.Index)
	anchor.MajorBlockIndex = majorBlockIndex

	return rootChain, nil
}

func (x *Executor) sendBlockAnchor(batch *database.Batch, anchor protocol.TransactionBody, sequenceNumber uint64, destPart string) error {
	destPartUrl := protocol.PartitionUrl(destPart)
	txn := new(protocol.Transaction)
	txn.Header.Principal = destPartUrl.JoinPath(protocol.AnchorPool)
	txn.Body = anchor

	// Create a synthetic origin signature
	initSig, err := new(signing.Builder).
		SetUrl(x.Describe.NodeUrl()).
		SetVersion(sequenceNumber).
		InitiateSynthetic(txn, destPartUrl)
	if err != nil {
		return errors.Wrap(errors.StatusInternalError, err)
	}

	// Create a key signature
	keySig, err := x.signTransaction(batch, txn, initSig.DestinationNetwork)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	// Send
	env := &protocol.Envelope{Transaction: []*protocol.Transaction{txn}, Signatures: []protocol.Signature{initSig, keySig}}
	err = x.dispatcher.BroadcastTx(context.Background(), txn.Header.Principal, env)
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
