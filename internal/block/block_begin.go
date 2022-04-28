package block

import (
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
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// BeginBlock implements ./Chain
func (x *Executor) BeginBlock(block *Block) (err error) {
	x.logDebug("Begin block", "height", block.Index, "leader", block.IsLeader, "time", block.Time)

	if block.IsLeader {
		// TODO Should we have more than just the leader do this for redundancy?

		// Use a read-only batch
		batch := block.Batch.Begin(false)
		defer batch.Discard()

		// Finalize the previous block
		err = x.finalizeBlock(block.Batch, uint64(block.Index))
		if err != nil {
			return err
		}
	}

	// Reset the block state
	err = indexing.BlockState(block.Batch, x.Network.NodeUrl(protocol.Ledger)).Clear()
	if err != nil {
		return err
	}

	// Load the ledger state
	ledger := block.Batch.Account(x.Network.NodeUrl(protocol.Ledger))
	var ledgerState *protocol.InternalLedger
	err = ledger.GetStateAs(&ledgerState)
	switch {
	case err == nil:
		// Make sure the block index is increasing
		if ledgerState.Index >= block.Index {
			panic(fmt.Errorf("Current height is %d but the next block height is %d!", ledgerState.Index, block.Index))
		}

	case x.isGenesis && errors.Is(err, storage.ErrNotFound):
		// OK

	default:
		return fmt.Errorf("cannot load ledger: %w", err)
	}

	// Reset transient values
	ledgerState.Index = block.Index
	ledgerState.Timestamp = block.Time

	// Reset the ledger's ACME burnt counter
	ledgerState.AcmeBurnt = *big.NewInt(0)

	err = ledger.PutState(ledgerState)
	if err != nil {
		return fmt.Errorf("cannot write ledger: %w", err)
	}

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

	return nil
}

func (x *Executor) captureValueAsDataEntry(batch *database.Batch, internalAccountPath string, value interface{}) error {
	if value == nil {
		return nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "cannot marshal value as json: %w", err)
	}

	wd := protocol.WriteData{}
	wd.Entry.Data = append(wd.Entry.Data, data)
	dataAccountUrl := x.Network.NodeUrl(internalAccountPath)

	var signer protocol.Signer
	signerUrl := x.Network.ValidatorPage(0)
	err = batch.Account(signerUrl).GetStateAs(&signer)
	if err != nil {
		return err
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = x.Network.NodeUrl()
	txn.Body = &wd
	txn.Header.Initiator = signerUrl.AccountID32()

	sw := protocol.SegWitDataEntry{}
	sw.Cause = *(*[32]byte)(txn.GetHash())
	sw.EntryHash = *(*[32]byte)(wd.Entry.Hash())
	sw.EntryUrl = txn.Header.Principal
	txn.Body = &sw

	st := chain.NewStateManager(batch.Begin(true), x.Network.NodeUrl(), signerUrl, signer, nil, txn, 0, x.logger)
	defer st.Discard()

	var da *protocol.DataAccount
	va := batch.Account(dataAccountUrl)
	err = va.GetStateAs(&da)
	if err != nil {
		return err
	}

	st.UpdateData(da, wd.Entry.Hash(), &wd.Entry)

	err = putSyntheticTransaction(
		batch, txn,
		&protocol.TransactionStatus{Delivered: true},
		&protocol.InternalSignature{Network: signerUrl})
	if err != nil {
		return err
	}

	_, err = st.Commit()
	return err
}

// finalizeBlock builds the block anchor and signs and sends synthetic
// transactions (including the block anchor) for the previously committed block.
func (x *Executor) finalizeBlock(batch *database.Batch, currentBlockIndex uint64) error {
	// Load the ledger state
	var ledgerState *protocol.InternalLedger
	err := batch.Account(x.Network.Ledger()).GetStateAs(&ledgerState)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load ledger: %w", err)
	}

	// If the last block is more than one block old, it has already been finalized
	if uint64(ledgerState.Index) < currentBlockIndex-1 {
		return nil
	}

	// Build receipts for synthetic transactions produced in the previous block
	err = x.sendSyntheticTransactions(batch)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "build synthetic transaction receipts: %w", err)
	}

	// Send the block anchor
	anchor, err := x.buildBlockAnchor(batch, ledgerState)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "build block anchor: %w", err)
	}

	switch x.Network.Type {
	case config.Directory:
		// DN -> all BVNs
		for _, bvn := range x.Network.GetBvnNames() {
			err = x.sendBlockAnchor(anchor, bvn)
			if err != nil {
				return errors.Wrap(errors.StatusUnknown, err)
			}
		}

	case config.BlockValidator:
		// BVN -> DN
		err = x.sendBlockAnchor(anchor, protocol.Directory)
		if err != nil {
			return errors.Wrap(errors.StatusUnknown, err)
		}
	}

	errs := x.dispatcher.Send(context.Background())
	go func() {
		for err := range errs {
			x.checkDispatchError(err)
		}
	}()

	return nil
}

func (x *Executor) sendSyntheticTransactions(batch *database.Batch) error {
	// Load the root chain's index chain's last two entries
	record := batch.Account(x.Network.Ledger())
	last, nextLast, err := indexing.LoadLastTwoIndexEntries(record, protocol.MinorRootIndexChain)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load root index chain's last two entries: %w", err)
	}
	if last == nil {
		return nil // Root chain is empty
	}
	rootAnchor := last.Source
	var prevRootAnchor uint64
	if nextLast != nil {
		prevRootAnchor = nextLast.Source + 1
	}

	// Load the synthetic transaction chain's index chain's last two entries
	last, nextLast, err = indexing.LoadLastTwoIndexEntries(record, protocol.IndexChain(protocol.SyntheticChain, false))
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load synthetic transaction index chain's last two entries: %w", err)
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

	// Load the root chain
	chain, err := batch.Account(x.Network.Ledger()).ReadChain(protocol.MinorRootChain)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load root chain: %w", err)
	}

	// Prove the synthetic transaction chain anchor
	rootProof, err := chain.Receipt(int64(synthAnchor), int64(rootAnchor))
	if err != nil {
		return errors.Format(errors.StatusUnknown, "prove from %d to %d on the root chain: %w", synthAnchor, rootAnchor, err)
	}

	// Load the synthetic transaction chain
	chain, err = batch.Account(x.Network.Ledger()).ReadChain(protocol.SyntheticChain)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load root chain: %w", err)
	}

	// Get transaction hashes
	hashes, err := chain.Entries(int64(synthStart), int64(synthEnd)+1)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load entries %d through %d of synthetic transaction chain: %w", synthStart, synthEnd, err)
	}

	// For each synthetic transaction from the last block
	for i, hash := range hashes {
		// Load it
		record := batch.Transaction(hash)
		state, err := record.GetState()
		if err != nil {
			return errors.Format(errors.StatusUnknown, "load synthetic transaction: %w", err)
		}
		txn := state.Transaction

		// TODO Can we make this less hacky?
		status, err := record.GetStatus()
		if err != nil {
			return errors.Format(errors.StatusUnknown, "load synthetic transaction status: %w", err)
		}
		sigs, err := getAllSignatures(batch, record, status, txn.Header.Initiator[:])
		if err != nil {
			return errors.Format(errors.StatusUnknown, "load synthetic transaction signatures: %w", err)
		}
		if len(sigs) == 0 {
			return errors.Format(errors.StatusInternalError, "synthetic transaction %X does not have a synthetic origin signature", hash[:4])
		}
		if len(sigs) > 1 {
			return errors.Format(errors.StatusInternalError, "synthetic transaction %X has more than one signature", hash[:4])
		}
		initSig := sigs[0]

		// Sign it
		keySig, err := new(signing.Builder).
			SetType(protocol.SignatureTypeED25519).
			SetPrivateKey(x.Key).
			SetKeyPageUrl(x.Network.ValidatorBook(), 0).
			SetVersion(1).
			SetTimestamp(1).
			Sign(hash)
		if err != nil {
			return errors.Format(errors.StatusUnknown, "sign synthetic transaction: %w", err)
		}

		// Prove it
		synthProof, err := chain.Receipt(int64(i+int(synthStart)), int64(synthEnd))
		if err != nil {
			return errors.Format(errors.StatusUnknown, "prove from %d to %d on the synthetic transaction chain: %w", i+int(synthStart), synthEnd, err)
		}

		r, err := managed.CombineReceipts(synthProof, rootProof)
		if err != nil {
			return errors.Format(errors.StatusUnknown, "combine receipts: %w", err)
		}

		proofSig := new(protocol.ReceiptSignature)
		proofSig.SourceNetwork = x.Network.NodeUrl()
		proofSig.TransactionHash = *(*[32]byte)(hash)
		proofSig.Receipt = *protocol.ReceiptFromManaged(r)

		// Record the proof signature but DO NOT record the key signature! Each
		// node has a different key, so recording the key signature here would
		// cause a consensus failure!
		err = batch.Transaction(proofSig.Hash()).PutState(&database.SigOrTxn{
			Hash:      proofSig.TransactionHash,
			Signature: proofSig,
		})
		if err != nil {
			return errors.Format(errors.StatusUnknown, "store signature: %w", err)
		}
		_, err = batch.Transaction(hash).AddSignature(proofSig)
		if err != nil {
			return errors.Format(errors.StatusUnknown, "record receipt for %X: %w", hash[:4], err)
		}

		env := &protocol.Envelope{Transaction: []*protocol.Transaction{txn}, Signatures: []protocol.Signature{initSig, keySig, proofSig}}
		err = x.dispatcher.BroadcastTx(context.Background(), txn.Header.Principal, env)
		if err != nil {
			return errors.Format(errors.StatusUnknown, "send synthetic transaction %X: %w", hash[:4], err)
		}
	}

	return nil
}

func (x *Executor) buildBlockAnchor(batch *database.Batch, ledgerState *protocol.InternalLedger) (*protocol.SyntheticAnchor, error) {
	// Load the root chain
	ledger := batch.Account(x.Network.Ledger())
	rootChain, err := ledger.ReadChain(protocol.MinorRootChain)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknown, "load root chain: %w", err)
	}

	// Load the list of chain updates
	updates, err := indexing.BlockChainUpdates(batch, &x.Network, uint64(ledgerState.Index)).Get()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknown, "load block chain updates index: %w", err)
	}

	anchor := new(protocol.SyntheticAnchor)
	anchor.Source = x.Network.NodeUrl()
	anchor.RootIndex = uint64(rootChain.Height()) - 1
	anchor.RootAnchor = *(*[32]byte)(rootChain.Anchor())
	anchor.Block = uint64(ledgerState.Index)
	anchor.AcmeBurnt = ledgerState.AcmeBurnt
	if x.Network.Type == config.Directory {
		anchor.AcmeOraclePrice = ledgerState.ActiveOracle
	}

	// TODO This is pretty inefficient; we're constructing a receipt for every
	// anchor. If we were more intelligent about it, we could send just the
	// Merkle state and a list of transactions, though we would need that for
	// the root chain and each anchor chain.

	anchorUrl := x.Network.NodeUrl(protocol.AnchorPool)
	record := batch.Account(anchorUrl)
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
			return nil, errors.Format(errors.StatusUnknown, "load minor index chain of intermediate anchor chain %s: %w", update.Name, err)
		}

		from, to, anchorIndex, err := getRangeFromIndexEntry(indexChain, uint64(indexChain.Height())-1)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknown, "load range from minor index chain of intermediate anchor chain %s: %w", update.Name, err)
		}

		rootReceipt, err := rootChain.Receipt(int64(anchorIndex), rootChain.Height()-1)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknown, "build receipt for the root chain: %w", err)
		}

		anchorChain, err := record.ReadChain(update.Name)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknown, "load intermediate anchor chain %s: %w", update.Name, err)
		}

		for i := from; i <= to; i++ {
			anchorReceipt, err := anchorChain.Receipt(int64(i), int64(to))
			if err != nil {
				return nil, errors.Format(errors.StatusUnknown, "build receipt for intermediate anchor chain %s: %w", update.Name, err)
			}

			receipt, err := anchorReceipt.Combine(rootReceipt)
			if err != nil {
				return nil, errors.Format(errors.StatusUnknown, "build receipt for intermediate anchor chain %s: %w", update.Name, err)
			}

			r := protocol.ReceiptFromManaged(receipt)
			anchor.Receipts = append(anchor.Receipts, *r)
		}
	}

	return anchor, nil
}

func (x *Executor) sendBlockAnchor(anchor *protocol.SyntheticAnchor, subnet string) error {
	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.SubnetUrl(subnet).JoinPath(protocol.AnchorPool)
	txn.Body = anchor

	// Create a synthetic origin signature
	initSig, err := new(signing.Builder).
		SetUrl(x.Network.NodeUrl()).
		SetVersion(anchor.Block).
		InitiateSynthetic(txn, x.Router)
	if err != nil {
		return errors.Wrap(errors.StatusInternalError, err)
	}

	// Create a key signature
	keySig, err := new(signing.Builder).
		SetType(protocol.SignatureTypeED25519).
		SetPrivateKey(x.Key).
		SetKeyPageUrl(x.Network.ValidatorBook(), 0).
		SetVersion(1).
		Sign(txn.GetHash())
	if err != nil {
		return errors.Wrap(errors.StatusInternalError, err)
	}

	// Send
	env := &protocol.Envelope{Transaction: []*protocol.Transaction{txn}, Signatures: []protocol.Signature{initSig, keySig}}
	err = x.dispatcher.BroadcastTx(context.Background(), txn.Header.Principal, env)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "send anchor for block %d: %w", anchor.Block, err)
	}

	return nil
}

// checkDispatchError returns nil if the error can be ignored.
func (x *Executor) checkDispatchError(err error) {
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

	var protoErr *protocol.Error
	if errors.As(err, &protoErr) {
		// Without this, the simulator generates tons of errors. Why?
		if protoErr.Code == protocol.ErrorCodeAlreadyDelivered {
			return
		}
	}

	// It's a real error
	x.logger.Error("Failed to dispatch transactions", "error", err)
}
