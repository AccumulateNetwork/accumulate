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

	// Process updates when present
	if len(body.Updates) > 0 && st.Network.Type != config.Directory {
		err := processNetworkAccountUpdates(st, tx, body.Updates)
		if err != nil {
			return nil, err
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

func processNetworkAccountUpdates(st *StateManager, delivery *Delivery, updates []protocol.NetworkAccountUpdate) error {
	for _, update := range updates {
		txn := new(protocol.Transaction)
		txn.Body = update.Body

		switch update.Name {
		case protocol.OperatorBook:
			txn.Header.Principal = st.DefaultOperatorPage()
		case protocol.Globals,
			protocol.Oracle,
			protocol.Network:
			txn.Header.Principal = st.NodeUrl(update.Name)
		default:
			return fmt.Errorf("unknown network account %q", update.Name)
		}

		st.State.ProcessAdditionalTransaction(delivery.NewInternal(txn))
	}
	return nil
}
