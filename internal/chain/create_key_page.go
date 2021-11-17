package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type CreateKeyPage struct{}

func (CreateKeyPage) Type() types.TxType { return types.TxTypeCreateKeyPage }

func (CreateKeyPage) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	var group *protocol.SigSpecGroup
	switch sponsor := st.Sponsor.(type) {
	case *state.AdiState:
		// Create an unbound sig spec
	case *protocol.SigSpecGroup:
		group = sponsor
	default:
		return fmt.Errorf("%v cannot sponsor a sig spec", sponsor.Header().Type)
	}

	body := new(protocol.CreateSigSpec)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	if len(body.Keys) == 0 {
		return fmt.Errorf("cannot create empty sig spec")
	}

	msUrl, err := url.Parse(body.Url)
	if err != nil {
		return fmt.Errorf("invalid target URL: %v", err)
	}

	if !msUrl.Identity().Equal(st.SponsorUrl.Identity()) {
		return fmt.Errorf("%q does not belong to %q", msUrl, st.SponsorUrl)
	}

	scc := new(protocol.SyntheticCreateChain)
	scc.Cause = types.Bytes(tx.TransactionHash()).AsBytes32()
	st.Submit(st.SponsorUrl, scc)

	spec := protocol.NewSigSpec()
	spec.ChainUrl = types.String(msUrl.String())

	if group != nil {
		groupUrl, err := group.ParseUrl()
		if err != nil {
			// Failing here would require writing an invalid URL to the state.
			// But stuff happens, so don't panic.
			return fmt.Errorf("invalid sponsor URL: %v", err)
		}

		group.SigSpecs = append(group.SigSpecs, types.Bytes(msUrl.ResourceChain()).AsBytes32())
		spec.SigSpecId = types.Bytes(groupUrl.ResourceChain()).AsBytes32()

		err = scc.Update(group)
		if err != nil {
			return fmt.Errorf("failed to marshal state: %v", err)
		}
	}

	for _, sig := range body.Keys {
		ss := new(protocol.KeySpec)
		ss.PublicKey = sig.PublicKey
		spec.Keys = append(spec.Keys, ss)
	}

	err = scc.Create(spec)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %v", err)
	}

	return nil
}
