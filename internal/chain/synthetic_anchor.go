package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
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
		st.blockState.DidReceiveDnAnchor(body)
	default:
		return nil, fmt.Errorf("invalid source: not a BVN or the DN")
	}

	// Update the oracle
	if fromDirectory {
		if body.AcmeOraclePrice == 0 {
			return nil, fmt.Errorf("attempting to set oracle price to 0")
		}

		ledgerState := protocol.NewInternalLedger()
		err := st.LoadUrlAs(st.nodeUrl.JoinPath(protocol.Ledger), ledgerState)
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

	return nil, nil
}
