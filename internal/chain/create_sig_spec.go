package chain

import (
	"bytes"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type CreateSigSpec struct{}

func (CreateSigSpec) Type() types.TxType { return types.TxTypeCreateSigSpec }

func checkCreateSigSpec(st *state.StateEntry, tx *transactions.GenTransaction) (*protocol.CreateSigSpec, *protocol.SigSpecGroup, *url.URL, error) {
	adiUrl, err := url.Parse(tx.SigInfo.URL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid sponsor URL: %v", err)
	}

	if st.ChainHeader == nil {
		return nil, nil, nil, fmt.Errorf("sponsor not found")
	}

	var group *protocol.SigSpecGroup
	switch st.ChainHeader.Type {
	case types.ChainTypeAdi:
		// Create an unbound sig spec
	case types.ChainTypeSigSpecGroup:
		group = new(protocol.SigSpecGroup)
		err = st.ChainState.As(group)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to decode sponsor: %v", err)
		}
	default:
		return nil, nil, nil, fmt.Errorf("%v cannot sponsor a sig spec", st.ChainHeader.Type)
	}

	body := new(protocol.CreateSigSpec)
	err = tx.As(body)
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

	if !bytes.Equal(adiUrl.IdentityChain(), msUrl.IdentityChain()) {
		return nil, nil, nil, fmt.Errorf("%q does not belong to %q", msUrl, adiUrl)
	}

	return body, group, msUrl, nil
}

func (CreateSigSpec) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	_, _, _, err := checkCreateSigSpec(st, tx)
	return err
}

func (CreateSigSpec) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	body, group, url, err := checkCreateSigSpec(st, tx)
	if err != nil {
		return nil, err
	}

	res := new(DeliverTxResult)
	spec := protocol.NewSigSpec()
	spec.ChainUrl = types.String(url.String())
	res.AddChain(spec)

	if group != nil {
		groupUrl, err := group.ParseUrl()
		if err != nil {
			// Failing here would require writing an invalid URL to the state.
			// But stuff happens, so don't panic.
			return nil, fmt.Errorf("invalid sponsor URL: %v", err)
		}

		res.AddChain(group)
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

	return res, nil
}
