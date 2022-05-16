package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Process the anchor from BVN -> DN

type PartitionAnchor struct{}

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
	name, ok := protocol.ParseSubnetUrl(body.Source)
	if !ok {
		return nil, fmt.Errorf("invalid source: not a BVN or the DN")
	}

	// Return ACME burnt by buying credits to the supply
	var issuerState *protocol.TokenIssuer
	err := st.LoadUrlAs(protocol.AcmeUrl(), &issuerState)
	if err != nil {
		return nil, fmt.Errorf("unable to load acme ledger")
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

	if body.Major {
		// TODO Handle major blocks?
		return nil, nil
	}

	return nil, nil
}
