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

type AssignSigSpecGroup struct{}

func (AssignSigSpecGroup) Type() types.TxType { return types.TxTypeAssignSigSpecGroup }

func checkAssignSigSpecGroup(st *state.StateEntry, tx *transactions.GenTransaction) (*url.URL, state.Chain, error) {
	sponsorUrl, err := url.Parse(tx.SigInfo.URL)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid sponsor URL: %v", err)
	}

	if st.ChainHeader == nil {
		return nil, nil, fmt.Errorf("sponsor not found")
	}

	body := new(protocol.AssignSigSpecGroup)
	err = tx.As(body)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid payload: %v", err)
	}

	ssgUrl, err := url.Parse(body.Url)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid target URL: %v", err)
	}

	if !bytes.Equal(sponsorUrl.IdentityChain(), ssgUrl.IdentityChain()) {
		return nil, nil, fmt.Errorf("%q and %q belong to different identities", sponsorUrl, ssgUrl)
	}

	ssg := new(protocol.SigSpecGroup)
	_, err = st.DB.LoadChainAs(ssgUrl.ResourceChain(), ssg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load key group: %v", err)
	}

	var chain state.Chain
	switch st.ChainHeader.Type {
	case types.ChainTypeAdi:
		chain = new(state.AdiState)
	case types.ChainTypeTokenAccount, types.ChainTypeAnonTokenAccount:
		chain = new(state.TokenAccount)
	default:
		return nil, nil, fmt.Errorf("cannot assign key group to %v", st.ChainHeader.Type)
	}

	err = st.ChainState.As(chain)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal sponsor: %v", err)
	}

	return ssgUrl, chain, nil
}

func (AssignSigSpecGroup) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	_, _, err := checkAssignSigSpecGroup(st, tx)
	return err
}

func (AssignSigSpecGroup) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	url, sponsor, err := checkAssignSigSpecGroup(st, tx)
	if err != nil {
		return nil, err
	}

	sigSpecId := types.Bytes(url.ResourceChain()).AsBytes32()
	switch sponsor := sponsor.(type) {
	case *state.AdiState:
		sponsor.SigSpecId = sigSpecId
	case *state.TokenAccount:
		sponsor.SigSpecId = sigSpecId
	default:
		// Should never happen, return an error anyways
		return nil, fmt.Errorf("internal error")
	}

	data, err := sponsor.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %v", err)
	}

	txHash := types.Bytes(tx.TransactionHash()).AsBytes32()
	st.DB.AddStateEntry(st.ChainId, &txHash, &state.Object{Entry: data})
	return new(DeliverTxResult), nil
}
