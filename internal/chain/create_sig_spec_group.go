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

type CreateSigSpecGroup struct{}

func (CreateSigSpecGroup) Type() types.TxType { return types.TxTypeCreateSigSpecGroup }

func checkCreateSigSpecGroup(st *state.StateEntry, tx *transactions.GenTransaction) ([]*protocol.MultiSigSpec, *url.URL, error) {
	adiUrl, err := url.Parse(tx.SigInfo.URL)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid sponsor URL: %v", err)
	}

	if st.ChainHeader == nil {
		return nil, nil, fmt.Errorf("sponsor not found")
	}

	if st.ChainHeader.Type != types.ChainTypeAdi {
		return nil, nil, fmt.Errorf("%q is not an ADI", tx.SigInfo.URL)
	}

	body := new(protocol.CreateSigSpecGroup)
	err = tx.As(body)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid payload: %v", err)
	}

	if len(body.MultiSigSpecs) == 0 {
		return nil, nil, fmt.Errorf("cannot create empty key group")
	}

	sgUrl, err := url.Parse(body.Url)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid target URL: %v", err)
	}

	if !bytes.Equal(adiUrl.IdentityChain(), sgUrl.IdentityChain()) {
		return nil, nil, fmt.Errorf("%q does not belong to %q", sgUrl, adiUrl)
	}

	entries := make([]*protocol.MultiSigSpec, len(body.MultiSigSpecs))
	for i, chainId := range body.MultiSigSpecs {
		entry := new(protocol.MultiSigSpec)
		_, err := st.DB.LoadChainAs(chainId[:], entry)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to fetch key set: %v", err)
		}

		u, err := entry.ParseUrl()
		if err != nil {
			// This should not happen. Only valid URLs should be stored.
			return nil, nil, fmt.Errorf("invalid key set state: bad URL: %v", err)
		}

		if !bytes.Equal(adiUrl.IdentityChain(), u.IdentityChain()) {
			return nil, nil, fmt.Errorf("%q does not belong to %q", u, adiUrl)
		}

		entries[i] = entry
	}

	return entries, sgUrl, nil
}

func (CreateSigSpecGroup) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	_, _, err := checkCreateSigSpecGroup(st, tx)
	return err
}

func (CreateSigSpecGroup) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	entries, url, err := checkCreateSigSpecGroup(st, tx)
	if err != nil {
		return nil, err
	}

	ssg := protocol.NewSigSpecGroup()
	ssg.ChainUrl = types.String(url.String())

	for _, mss := range entries {
		u, err := mss.ParseUrl()
		if err != nil {
			// We already did this, so this should never fail here.
			return nil, fmt.Errorf("invalid key set state: bad URL: %v", err)
		}

		chainId := types.Bytes(u.ResourceChain()).AsBytes32()
		ssg.MultiSigSpecs = append(ssg.MultiSigSpecs, chainId)
	}

	data, err := ssg.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %v", err)
	}

	chainId := types.Bytes(url.ResourceChain()).AsBytes32()
	txHash := types.Bytes(tx.TransactionHash()).AsBytes32()
	st.DB.AddStateEntry(&chainId, &txHash, &state.Object{Entry: data})
	return new(DeliverTxResult), nil
}
