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

func checkCreateSigSpec(st *state.StateEntry, tx *transactions.GenTransaction) (*protocol.CreateSigSpec, *url.URL, error) {
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

	body := new(protocol.CreateSigSpec)
	err = tx.As(body)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid payload: %v", err)
	}

	if len(body.Keys) == 0 {
		return nil, nil, fmt.Errorf("cannot create empty sig spec")
	}

	msUrl, err := url.Parse(body.Url)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid target URL: %v", err)
	}

	if !bytes.Equal(adiUrl.IdentityChain(), msUrl.IdentityChain()) {
		return nil, nil, fmt.Errorf("%q does not belong to %q", msUrl, adiUrl)
	}

	return body, msUrl, nil
}

func (CreateSigSpec) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	_, _, err := checkCreateSigSpec(st, tx)
	return err
}

func (CreateSigSpec) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	body, url, err := checkCreateSigSpec(st, tx)
	if err != nil {
		return nil, err
	}

	mss := protocol.NewSigSpec()
	mss.ChainUrl = types.String(url.String())

	for _, sig := range body.Keys {
		ss := new(protocol.KeySpec)
		ss.HashAlgorithm = sig.HashAlgorithm
		ss.KeyAlgorithm = sig.KeyAlgorithm
		ss.PublicKey = sig.PublicKey
		mss.Keys = append(mss.Keys, ss)
	}

	data, err := mss.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %v", err)
	}

	chainId := types.Bytes(url.ResourceChain()).AsBytes32()
	txHash := types.Bytes(tx.TransactionHash()).AsBytes32()
	st.DB.AddStateEntry(&chainId, &txHash, &state.Object{Entry: data})
	return new(DeliverTxResult), nil
}
