package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type CreateSigSpec struct{}

func (CreateSigSpec) Type() types.TxType { return types.TxTypeCreateSigSpec }

func checkCreateSigSpec(st *StateManager, tx *transactions.GenTransaction) (*protocol.CreateSigSpec, *protocol.SigSpecGroup, *url.URL, error) {
	var group *protocol.SigSpecGroup
	switch sponsor := st.Sponsor.(type) {
	case *state.AdiState:
		// Create an unbound sig spec
	case *protocol.SigSpecGroup:
		group = sponsor
	default:
		return nil, nil, nil, fmt.Errorf("%v cannot sponsor a sig spec", sponsor.Header().Type)
	}

	body := new(protocol.CreateSigSpec)
	err := tx.As(body)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid payload: %v", err)
	}

	if len(body.Keys) == 0 {
		return nil, nil, nil, fmt.Errorf("cannot create empty sig spec")
	}

	msUrl, err := url.Parse(body.Url)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid target URL: %v", err)
	}

	if !msUrl.Identity().Equal(st.SponsorUrl.Identity()) {
		return nil, nil, nil, fmt.Errorf("%q does not belong to %q", msUrl, st.SponsorUrl)
	}

	return body, group, msUrl, nil
}

func (CreateSigSpec) CheckTx(st *StateManager, tx *transactions.GenTransaction) error {
	_, _, _, err := checkCreateSigSpec(st, tx)
	return err
}

func (CreateSigSpec) DeliverTx(st *StateManager, tx *transactions.GenTransaction) error {
	body, group, url, err := checkCreateSigSpec(st, tx)
	if err != nil {
		return err
	}

	spec := protocol.NewSigSpec()
	spec.ChainUrl = types.String(url.String())
	st.Store(spec)

	if group != nil {
		groupUrl, err := group.ParseUrl()
		if err != nil {
			// Failing here would require writing an invalid URL to the state.
			// But stuff happens, so don't panic.
			return fmt.Errorf("invalid sponsor URL: %v", err)
		}

		st.Store(group)
		group.SigSpecs = append(group.SigSpecs, types.Bytes(url.ResourceChain()).AsBytes32())
		spec.SigSpecId = types.Bytes(groupUrl.ResourceChain()).AsBytes32()
	}

	for _, sig := range body.Keys {
		ss := new(protocol.KeySpec)
		ss.HashAlgorithm = sig.HashAlgorithm
		ss.KeyAlgorithm = sig.KeyAlgorithm
		ss.PublicKey = sig.PublicKey
		spec.Keys = append(spec.Keys, ss)
	}

	return nil
}
