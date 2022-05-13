package chain

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Process the anchor from DN -> BVN

type DirectoryAnchor struct {
	Network *config.Network
}

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
	if _, ok := st.Origin.(*protocol.Anchor); !ok {
		return nil, fmt.Errorf("invalid origin record: want %v, got %v", protocol.AccountTypeAnchor, st.Origin.Type())
	}

	// Verify the source URL is from the DN
	if !protocol.IsDnUrl(body.Source) {
		return nil, fmt.Errorf("invalid source: not the DN")
	}

	if body.AcmeOraclePrice != 0 {
		var ledgerState *protocol.InternalLedger
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
	err := st.AddChainEntry(st.OriginUrl, protocol.AnchorChain(protocol.Directory), protocol.ChainTypeAnchor, body.RootAnchor[:], body.RootIndex, body.Block)
	if err != nil {
		return nil, err
	}

	// And the BPT root
	err = st.AddChainEntry(st.OriginUrl, protocol.AnchorChain(protocol.Directory)+"-bpt", protocol.ChainTypeAnchor, body.StateRoot[:], 0, 0)
	if err != nil {
		return nil, err
	}

	if body.Major {
		// TODO Handle major blocks?
		return nil, nil
	}

	// Process OperatorUpdates when present
	if len(body.OperatorUpdates) > 0 {
		result, err := executeOperatorUpdates(st, body)
		if err != nil {
			return result, err
		}
	}

	// Process receipts
	for i, receipt := range body.Receipts {
		receipt := receipt // See docs/developer/rangevarref.md
		if !bytes.Equal(receipt.Result, body.RootAnchor[:]) {
			return nil, fmt.Errorf("receipt %d is invalid: result does not match the anchor", i)
		}

		st.logger.Debug("Received receipt", "from", logging.AsHex(receipt.Start), "to", logging.AsHex(body.RootAnchor), "block", body.Block, "source", body.Source, "module", "synthetic")

		synth, err := st.batch.Account(st.Ledger()).SyntheticForAnchor(*(*[32]byte)(receipt.Start))
		if err != nil {
			return nil, fmt.Errorf("failed to load pending synthetic transactions for anchor %X: %w", receipt.Start[:4], err)
		}
		for _, hash := range synth {
			d := tx.NewSyntheticReceipt(hash, body.Source, &receipt)
			st.state.ProcessAdditionalTransaction(d)
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
