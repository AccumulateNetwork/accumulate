package chain

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Process the anchor from BVN -> DN

type PartitionAnchor struct {
	Network *config.Network
}

func (PartitionAnchor) Type() protocol.TransactionType {
	return protocol.TransactionTypePartitionAnchor
}

func (PartitionAnchor) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (PartitionAnchor{}).Validate(st, tx)
}

func (x PartitionAnchor) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	// Unpack the payload
	body, ok := tx.Transaction.Body.(*protocol.PartitionAnchor)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.PartitionAnchor), tx.Transaction.Body)
	}

	// Verify the origin
	if _, ok := st.Origin.(*protocol.Anchor); !ok {
		return nil, fmt.Errorf("invalid origin record: want %v, got %v", protocol.AccountTypeAnchor, st.Origin.Type())
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

	// Return ACME burnt by buying credits to the supply
	var issuerState *protocol.TokenIssuer
	err := st.LoadUrlAs(protocol.AcmeUrl(), &issuerState)
	if err != nil {
		return nil, fmt.Errorf("unable to load acme ledger")
	}
	var ledgerState *protocol.InternalLedger
	err = st.LoadUrlAs(st.NodeUrl(protocol.Ledger), &ledgerState)
	if err != nil {
		return nil, fmt.Errorf("unable to load main ledger: %w", err)
	}
	issuerState.Issued.Sub(&issuerState.Issued, &body.AcmeBurnt)
	err = st.Update(issuerState)
	if err != nil {
		return nil, fmt.Errorf("failed to update issuer state: %v", err)
	}

	// Add the anchor to the chain - use the subnet name as the chain name
	err = st.AddChainEntry(st.OriginUrl, protocol.AnchorChain(name), protocol.ChainTypeAnchor, body.RootAnchor[:], body.RootIndex, body.Block)
	if err != nil {
		return nil, err
	}

	// And the BPT root
	err = st.AddChainEntry(st.OriginUrl, protocol.AnchorChain(name)+"-bpt", protocol.ChainTypeAnchor, body.StateRoot[:], 0, 0)
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
	for i, receipt := range body.Receipts {
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
