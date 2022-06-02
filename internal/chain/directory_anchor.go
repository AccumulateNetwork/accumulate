package chain

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Process the anchor from DN -> BVN

type DirectoryAnchor struct{}

func (DirectoryAnchor) Type() protocol.TransactionType {
	return protocol.TransactionTypeDirectoryAnchor
}

func (DirectoryAnchor) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (DirectoryAnchor{}).Validate(st, tx)
}

func (x DirectoryAnchor) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	// Unpack the payload
	body, ok := tx.Transaction.Body.(*protocol.DirectoryAnchor)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.DirectoryAnchor), tx.Transaction.Body)
	}

	// Verify the origin
	if _, ok := st.Origin.(*protocol.AnchorLedger); !ok {
		return nil, fmt.Errorf("invalid principal: want %v, got %v", protocol.AccountTypeAnchorLedger, st.Origin.Type())
	}

	// Verify the source URL is from the DN
	if !protocol.IsDnUrl(body.Source) {
		return nil, fmt.Errorf("invalid source: not the DN")
	}

	// Trigger a major block?
	if st.Network.Type != config.Directory {
		st.State.MakeMajorBlock = body.MakeMajorBlock
	}

	// Update the oracle
	if body.AcmeOraclePrice != 0 && st.Network.Type != config.Directory {
		var ledgerState *protocol.SystemLedger
		err := st.LoadUrlAs(st.NodeUrl(protocol.Ledger), &ledgerState)
		if err != nil {
			return nil, fmt.Errorf("unable to load main ledger: %w", err)
		}
		ledgerState.PendingOracle = body.AcmeOraclePrice
		err = st.Update(ledgerState)
		if err != nil {
			return nil, fmt.Errorf("failed to update ledger state: %v", err)
		}
	}

	// Add the anchor to the chain - use the subnet name as the chain name
	err := st.AddChainEntry(st.OriginUrl, protocol.RootAnchorChain(protocol.Directory), protocol.ChainTypeAnchor, body.RootChainAnchor[:], body.RootChainIndex, body.MinorBlockIndex)
	if err != nil {
		return nil, err
	}

	// And the BPT root
	err = st.AddChainEntry(st.OriginUrl, protocol.BPTAnchorChain(protocol.Directory), protocol.ChainTypeAnchor, body.StateTreeAnchor[:], 0, 0)
	if err != nil {
		return nil, err
	}

	if body.MajorBlockIndex > 0 {
		// TODO Handle major blocks?
		return nil, nil
	}

	// Process OperatorUpdates when present
	if len(body.OperatorUpdates) > 0 && st.Network.Type != config.Directory {
		result, err := executeOperatorUpdates(st, body)
		if err != nil {
			return result, err
		}
	}

	// Process receipts
	for i, receipt := range body.Receipts {
		receipt := receipt // See docs/developer/rangevarref.md
		if !bytes.Equal(receipt.Anchor, body.RootChainAnchor[:]) {
			return nil, fmt.Errorf("receipt %d is invalid: result does not match the anchor", i)
		}

		st.logger.Debug("Received receipt", "from", logging.AsHex(receipt.Start), "to", logging.AsHex(body.RootChainAnchor), "block", body.MinorBlockIndex, "source", body.Source, "module", "synthetic")

		synth, err := st.batch.Account(st.Ledger()).SyntheticForAnchor(*(*[32]byte)(receipt.Start))
		if err != nil {
			return nil, fmt.Errorf("failed to load pending synthetic transactions for anchor %X: %w", receipt.Start[:4], err)
		}
		for _, hash := range synth {
			d := tx.NewSyntheticReceipt(hash, body.Source, &receipt)
			st.State.ProcessAdditionalTransaction(d)
		}
	}

	return nil, nil
}

func executeOperatorUpdates(st *StateManager, body *protocol.DirectoryAnchor) (protocol.TransactionResult, error) {
	for _, opUpd := range body.OperatorUpdates {
		bookUrl := st.NodeUrl().JoinPath(protocol.OperatorBook)

		var page1 *protocol.KeyPage
		err := st.LoadUrlAs(protocol.FormatKeyPageUrl(bookUrl, 0), &page1)
		if err != nil {
			return nil, fmt.Errorf("invalid key page: %v", err)
		}

		// Validate source
		_, _, ok := page1.EntryByDelegate(body.Source.JoinPath(protocol.OperatorBook))
		if !ok {
			return nil, fmt.Errorf("source is not from DN but from %q", body.Source)
		}

		var page2 *protocol.KeyPage
		err = st.LoadUrlAs(protocol.FormatKeyPageUrl(bookUrl, 1), &page2)
		if err != nil {
			return nil, fmt.Errorf("invalid key page: %v", err)
		}

		updateKeyPage := &UpdateKeyPage{}
		err = updateKeyPage.executeOperation(page2, opUpd)
		if err != nil {
			return nil, fmt.Errorf("updateKeyPage operation failed: %w", err)
		}
		err = st.Update(page2)
		if err != nil {
			return nil, fmt.Errorf("unable to update main ledger: %w", err)
		}
	}
	return nil, nil
}
