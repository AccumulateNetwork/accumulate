package block

import (
	"encoding/json"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// BeginBlock implements ./Chain
func (m *Executor) BeginBlock(block *Block) (err error) {
	m.logDebug("Begin block", "height", block.Index, "leader", block.IsLeader, "time", block.Time)

	// Reset the block state
	err = indexing.BlockState(block.Batch, m.Network.NodeUrl(protocol.Ledger)).Clear()
	if err != nil {
		return err
	}

	// Load the ledger state
	ledger := block.Batch.Account(m.Network.NodeUrl(protocol.Ledger))
	var ledgerState *protocol.InternalLedger
	err = ledger.GetStateAs(&ledgerState)
	switch {
	case err == nil:
		// Make sure the block index is increasing
		if ledgerState.Index >= block.Index {
			panic(fmt.Errorf("Current height is %d but the next block height is %d!", ledgerState.Index, block.Index))
		}

	case m.isGenesis && errors.Is(err, storage.ErrNotFound):
		// OK

	default:
		return fmt.Errorf("cannot load ledger: %w", err)
	}

	// Reset transient values
	ledgerState.Index = block.Index
	ledgerState.Timestamp = block.Time

	err = ledger.PutState(ledgerState)
	if err != nil {
		return fmt.Errorf("cannot write ledger: %w", err)
	}

	//store votes from previous block, choosing to marshal as json to make it easily viewable by explorers
	data, err := json.Marshal(block.CommitInfo)
	if err != nil {
		m.logger.Error("cannot marshal voting info data as json")
	} else {
		wd := protocol.WriteData{}
		wd.Entry.Data = append(wd.Entry.Data, data)

		err := m.processInternalDataTransaction(block, protocol.Votes, &wd)
		if err != nil {
			m.logger.Error(fmt.Sprintf("error processing internal vote transaction, %v", err))
		}
	}

	//capture evidence of maleficence if any occurred
	if block.Evidence != nil {
		data, err := json.Marshal(block.Evidence)
		if err != nil {
			m.logger.Error("cannot marshal evidence as json")
		} else {
			wd := protocol.WriteData{}
			wd.Entry.Data = append(wd.Entry.Data, data)

			err := m.processInternalDataTransaction(block, protocol.Evidence, &wd)
			if err != nil {
				m.logger.Error(fmt.Sprintf("error processing internal evidence transaction, %v", err))
			}
		}
	}

	err = m.buildSyntheticTransactionReceipts(block)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "build synthetic transaction receipts: %w", err)
	}

	return nil
}

func (m *Executor) processInternalDataTransaction(block *Block, internalAccountPath string, wd *protocol.WriteData) error {
	dataAccountUrl := m.Network.NodeUrl(internalAccountPath)

	if wd == nil {
		return fmt.Errorf("no internal data transaction provided")
	}

	var signer protocol.Signer
	signerUrl := m.Network.ValidatorPage(0)
	err := block.Batch.Account(signerUrl).GetStateAs(&signer)
	if err != nil {
		return err
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = m.Network.NodeUrl()
	txn.Body = wd
	txn.Header.Initiator = signerUrl.AccountID32()

	sw := protocol.SegWitDataEntry{}
	sw.Cause = *(*[32]byte)(txn.GetHash())
	sw.EntryHash = *(*[32]byte)(wd.Entry.Hash())
	sw.EntryUrl = txn.Header.Principal
	txn.Body = &sw

	st := chain.NewStateManager(block.Batch.Begin(true), m.Network.NodeUrl(), signerUrl, signer, nil, txn, m.logger)
	defer st.Discard()

	var da *protocol.DataAccount
	va := block.Batch.Account(dataAccountUrl)
	err = va.GetStateAs(&da)
	if err != nil {
		return err
	}

	st.UpdateData(da, wd.Entry.Hash(), &wd.Entry)

	err = putSyntheticTransaction(
		block.Batch, txn,
		&protocol.TransactionStatus{Delivered: true},
		&protocol.InternalSignature{Network: signerUrl})
	if err != nil {
		return err
	}

	_, err = st.Commit()
	return err
}

func (m *Executor) buildSyntheticTransactionReceipts(block *Block) error {
	// Load the root chain's index chain
	chain, err := block.Batch.Account(m.Network.Ledger()).ReadChain(protocol.MinorRootIndexChain)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load root index chain: %w", err)
	}
	if chain.Height() == 0 {
		return errors.Format(errors.StatusUnknown, "root index chain is empty")
	}

	// Get the latest entry
	entry := new(protocol.IndexEntry)
	err = chain.EntryAs(chain.Height()-1, entry)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load entry %d of root index chain: %w", chain.Height()-1, err)
	}
	rootAnchor := entry.Source

	// Load the synthetic transaction chain's index chain
	chain, err = block.Batch.Account(m.Network.Ledger()).ReadIndexChain(protocol.SyntheticChain, false)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load synthetic transaction index chain: %w", err)
	}
	if chain.Height() == 0 {
		return nil // Nothing to do
	}

	// Get the latest entry
	entry = new(protocol.IndexEntry)
	err = chain.EntryAs(chain.Height()-1, entry)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load entry %d of synthetic transaction index chain: %w", chain.Height()-1, err)
	}
	synthEnd, synthAnchor := entry.Source, entry.Anchor

	// Get the previous entry
	var synthStart uint64
	if chain.Height() > 1 {
		entry = new(protocol.IndexEntry)
		err = chain.EntryAs(chain.Height()-2, entry)
		if err != nil {
			return errors.Format(errors.StatusUnknown, "load entry %d of synthetic transaction index chain: %w", chain.Height()-2, err)
		}
		synthStart = entry.Source + 1
	}

	// Load the root chain
	chain, err = block.Batch.Account(m.Network.Ledger()).ReadChain(protocol.MinorRootChain)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load root chain: %w", err)
	}

	// Prove the synthetic transaction chain anchor
	rootProof, err := chain.Receipt(int64(synthAnchor), int64(rootAnchor))
	if err != nil {
		return errors.Format(errors.StatusUnknown, "prove from %d to %d on the root chain: %w", synthAnchor, rootAnchor, err)
	}

	// Load the synthetic transaction chain
	chain, err = block.Batch.Account(m.Network.Ledger()).ReadChain(protocol.SyntheticChain)
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
		// Prove the synthetic transaction
		synthProof, err := chain.Receipt(int64(i+int(synthStart)), int64(synthEnd))
		if err != nil {
			return errors.Format(errors.StatusUnknown, "prove from %d to %d on the synthetic transaction chain: %w", i+int(synthStart), synthEnd, err)
		}

		r, err := managed.CombineReceipts(synthProof, rootProof)
		if err != nil {
			return errors.Format(errors.StatusUnknown, "combine receipts: %w", err)
		}

		sig := new(protocol.ReceiptSignature)
		sig.SourceNetwork = m.Network.NodeUrl()
		sig.TransactionHash = *(*[32]byte)(hash)
		sig.Receipt = *protocol.ReceiptFromManaged(r)

		err = block.Batch.Transaction(sig.Hash()).PutState(&database.SigOrTxn{
			Hash:      sig.TransactionHash,
			Signature: sig,
		})
		if err != nil {
			return errors.Format(errors.StatusUnknown, "store signature: %w", err)
		}

		_, err = block.Batch.Transaction(hash).AddSignature(sig)
		if err != nil {
			return errors.Format(errors.StatusUnknown, "record receipt for %X: %w", hash[:4], err)
		}
	}

	return nil
}
