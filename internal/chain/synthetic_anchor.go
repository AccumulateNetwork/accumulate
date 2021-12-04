package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type SyntheticAnchor struct {
	IsDirectory bool
}

func (SyntheticAnchor) Type() types.TxType { return types.TxTypeSyntheticAnchor }

func (x SyntheticAnchor) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	nodeUrl, err := nodeUrl(st.dbTx.DB(), x.IsDirectory)
	if err != nil {
		return fmt.Errorf("failed to retrieve subnet ID: %v", err)
	}

	// Verify that the sponsor is the node
	if !st.SponsorUrl.Equal(nodeUrl) {
		return fmt.Errorf("invalid sponsor: %q != %q", st.SponsorUrl, nodeUrl)
	}

	// Unpack the payload
	body := new(protocol.SyntheticAnchor)
	err = tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	// TODO Enable once mirroring has been implemented
	if false {
		// Load the source's record
		source, err := st.LoadString(body.Source)
		if err != nil {
			return fmt.Errorf("invalid source %q: %v", body.Source, err)
		}

		// Check that the source is an ADI
		if source.Header().Type != types.ChainTypeIdentity {
			return fmt.Errorf("invalid source %q: want chain type %v, got %v", body.Source, types.ChainTypeIdentity, source.Header().Type)
		}
	}

	chain := new(state.Anchor)
	chain.ChainUrl = types.String(nodeUrl.JoinPath(anchorChainName(x.IsDirectory, body.Major)).String())
	chain.Index = body.Index
	chain.Timestamp = body.Timestamp
	chain.Root = body.Root
	chain.ChainAnchor = body.ChainAnchor
	chain.Synthetic = body.SynthTxnAnchor
	chain.Chains = body.Chains
	st.Update(chain)
	return nil
}
