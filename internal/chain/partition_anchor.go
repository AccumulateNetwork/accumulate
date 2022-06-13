package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Process the anchor from BVN -> DN

type PartitionAnchor struct{}

func (PartitionAnchor) Type() protocol.TransactionType {
	return protocol.TransactionTypeBlockValidatorAnchor
}

func (PartitionAnchor) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (PartitionAnchor{}).Validate(st, tx)
}

func (x PartitionAnchor) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	// Unpack the payload
	body, ok := tx.Transaction.Body.(*protocol.BlockValidatorAnchor)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.BlockValidatorAnchor), tx.Transaction.Body)
	}

	// Verify the origin
	ledger, ok := st.Origin.(*protocol.AnchorLedger)
	if !ok {
		return nil, fmt.Errorf("invalid principal: want %v, got %v", protocol.AccountTypeAnchorLedger, st.Origin.Type())
	}

	// Verify the source URL and get the partition name
	name, ok := protocol.ParsePartitionUrl(body.Source)
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

	// Add the anchor to the chain - use the partition name as the chain name
	err = st.AddChainEntry(st.OriginUrl, protocol.RootAnchorChain(name), protocol.ChainTypeAnchor, body.RootChainAnchor[:], body.RootChainIndex, body.MinorBlockIndex)
	if err != nil {
		return nil, err
	}

	// And the BPT root
	err = st.AddChainEntry(st.OriginUrl, protocol.BPTAnchorChain(name), protocol.ChainTypeAnchor, body.StateTreeAnchor[:], 0, 0)
	if err != nil {
		return nil, err
	}

	// Did the partition complete a major block?
	if body.MajorBlockIndex > 0 {
		found := -1
		for i, u := range ledger.PendingMajorBlockAnchors {
			if u.Equal(body.Source) {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, errors.Format(errors.StatusInternalError, "partition %v is not in the pending list", body.Source)
		}
		ledger.PendingMajorBlockAnchors = append(ledger.PendingMajorBlockAnchors[:found], ledger.PendingMajorBlockAnchors[found+1:]...)
		err = st.Update(ledger)
		if err != nil {
			return nil, err
		}

		// If every partition has done the major block thing, do the major block
		// thing on the DN
		if len(ledger.PendingMajorBlockAnchors) == 0 {
			st.logger.Info("Completed major block", "major-index", ledger.MajorBlockIndex, "minor-index", body.MinorBlockIndex)
			st.State.MakeMajorBlock = ledger.MajorBlockIndex
		}
		return nil, nil
	}

	return nil, nil
}
