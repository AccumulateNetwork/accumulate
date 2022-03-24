package chain

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticAnchor struct {
	Network *config.Network
}

func (SyntheticAnchor) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticAnchor
}

func (x SyntheticAnchor) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	// Unpack the payload
	body, ok := tx.Transaction.Body.(*protocol.SyntheticAnchor)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticAnchor), tx.Transaction.Body)
	}

	// Verify the origin
	if _, ok := st.Origin.(*protocol.Anchor); !ok {
		return nil, fmt.Errorf("invalid origin record: want %v, got %v", protocol.AccountTypeAnchor, st.Origin.GetType())
	}

	// Verify the source URL and get the subnet name
	name, ok := protocol.ParseBvnUrl(body.Source)
	var fromDirectory bool
	switch {
	case ok:
	case protocol.IsDnUrl(body.Source):
		name, fromDirectory = protocol.Directory, true
	default:
		return nil, fmt.Errorf("invalid source: not a BVN or the DN")
	}

	// Update the oracle
	if fromDirectory {
		if body.AcmeOraclePrice == 0 {
			return nil, fmt.Errorf("attempting to set oracle price to 0")
		}

		var ledgerState *protocol.InternalLedger
		err := st.LoadUrlAs(st.nodeUrl.JoinPath(protocol.Ledger), &ledgerState)
		if err != nil {
			return nil, fmt.Errorf("unable to load main ledger: %w", err)
		}

		ledgerState.PendingOracle = body.AcmeOraclePrice
		st.Update(ledgerState)
	}

	// Add the anchor to the chain - use the subnet name as the chain name
	err := st.AddChainEntry(st.OriginUrl, protocol.AnchorChain(name), protocol.ChainTypeAnchor, body.RootAnchor[:], body.RootIndex, body.Block)
	if err != nil {
		return nil, err
	}

	if !fromDirectory {
		// The directory does not process receipts
		return nil, nil
	}

	if body.Major {
		// TODO Handle major blocks?
		return nil, nil
	}

	// Process receipts
	anchors := map[[32]byte]*protocol.Receipt{}
	for i, receipt := range body.Receipts {
		if !bytes.Equal(receipt.Result, body.RootAnchor[:]) {
			return nil, fmt.Errorf("receipt %d is invalid: result does not match the anchor", i)
		}

		st.logger.Debug("Received receipt", "from", logging.AsHex(receipt.Start), "to", logging.AsHex(body.RootAnchor), "block", body.Block, "source", body.Source, "module", "synthetic")
		anchors[*(*[32]byte)(receipt.Start)] = &body.Receipts[i]
	}

	var synthLedgerState *protocol.InternalSyntheticLedger
	err = st.LoadUrlAs(st.nodeUrl.JoinPath(protocol.SyntheticLedgerPath), &synthLedgerState)
	if err != nil {
		return nil, fmt.Errorf("unable to load synthetic transaction ledger: %w", err)
	}

	rootIndexChain, err := st.ReadChain(st.nodeUrl.JoinPath(protocol.Ledger), protocol.MinorRootIndexChain)
	if err != nil {
		return nil, fmt.Errorf("unable to load root index chain: %w", err)
	}

	synthIndexChain, err := st.ReadChain(st.nodeUrl.JoinPath(protocol.Ledger), protocol.IndexChain(protocol.SyntheticChain, false))
	if err != nil {
		return nil, fmt.Errorf("unable to load synthetic transaction index chain: %w", err)
	}

	rootChain, err := st.ReadChain(st.nodeUrl.JoinPath(protocol.Ledger), protocol.MinorRootChain)
	if err != nil {
		return nil, fmt.Errorf("unable to load root chain: %w", err)
	}

	synthChain, err := st.ReadChain(st.nodeUrl.JoinPath(protocol.Ledger), protocol.SyntheticChain)
	if err != nil {
		return nil, fmt.Errorf("unable to load synthetic transaction chain: %w", err)
	}

	for _, synth := range synthLedgerState.Pending {
		// Skip any entries that already have a receipt
		if !synth.NeedsReceipt {
			continue
		}

		// Do we have a receipt for this anchor?
		dirReceipt, ok := anchors[synth.RootAnchor]
		if !ok {
			continue
		}

		// TODO Save the receipts for transaction proofs in the future?

		// Load the root index entry
		rootEntry := new(protocol.IndexEntry)
		err = rootIndexChain.EntryAs(int64(synth.RootIndexIndex), rootEntry)
		if err != nil {
			return nil, fmt.Errorf("unable to load entry %d of the root index chain: %w", synth.RootIndexIndex, err)
		}

		// Load the synth index entry
		synthEntry := new(protocol.IndexEntry)
		err = synthIndexChain.EntryAs(int64(synth.SynthIndexIndex), synthEntry)
		if err != nil {
			return nil, fmt.Errorf("unable to load entry %d of the synthetic transaction index chain: %w", synth.SynthIndexIndex, err)
		}

		// Get the receipt from the synth anchor to the root anchor
		rootReceipt, err := rootChain.Receipt(int64(synthEntry.Anchor), int64(rootEntry.Source))
		if err != nil {
			return nil, fmt.Errorf("unable to build a receipt for the root chain: %w", err)
		}

		// Get the receipt from the synth txn to the synth anchor
		synthReceipt, err := synthChain.Receipt(int64(synth.SynthIndex), int64(synthEntry.Source))
		if err != nil {
			return nil, fmt.Errorf("unable to build a receipt for the synthetic transaction chain: %w", err)
		}

		// Combine all of the receipts, from the txn to the synth anchor to the
		// root anchor to the directory anchor
		receipt, err := combineReceipts(nil, synthReceipt, rootReceipt, dirReceipt.Convert())
		if err != nil {
			return nil, err
		}

		sig := new(protocol.ReceiptSignature)
		sig.Receipt = *protocol.ReceiptFromManaged(receipt)
		st.SignTransaction(synth.TransactionHash[:], sig)
		synth.NeedsReceipt = false
		st.logger.Debug("Built receipt for synthetic transaction", "hash", logging.AsHex(synth.TransactionHash), "block", body.Block, "module", "synthetic")
	}

	st.Update(synthLedgerState)
	return nil, nil
}
