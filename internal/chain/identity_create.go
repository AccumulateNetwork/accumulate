package chain

import (
	"fmt"
	"strings"

	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type IdentityCreate struct{}

func (IdentityCreate) Type() types.TxType { return types.TxTypeIdentityCreate }

func checkIdentityCreate(st *state.StateEntry, tx *transactions.GenTransaction) (*api.ADI, *url.URL, state.Chain, error) {
	if st.ChainHeader == nil {
		return nil, nil, nil, fmt.Errorf("sponsor not found")
	}

	body := new(api.ADI)
	err := tx.As(body)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid payload: %v", err)
	}

	identityUrl, err := url.Parse(*body.URL.AsString())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid URL: %v", err)
	}
	if identityUrl.Path != "" {
		return nil, nil, nil, fmt.Errorf("creating sub-ADIs is not supported")
	}
	if strings.ContainsRune(identityUrl.Hostname(), '.') {
		return nil, nil, nil, fmt.Errorf("ADI URLs cannot contain dots")
	}

	var sponsor state.Chain
	switch st.ChainHeader.Type {
	case types.ChainTypeAnonTokenAccount:
		sponsor = new(protocol.AnonTokenAccount)

	case types.ChainTypeAdi:
		sponsor = new(state.AdiState)

	default:
		return nil, nil, nil, fmt.Errorf("chain type %d cannot sponsor ADIs", st.ChainHeader.Type)
	}

	err = st.ChainState.As(sponsor)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to decode sponsor: %v", err)
	}

	return body, identityUrl, sponsor, nil
}

func (IdentityCreate) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	_, _, _, err := checkIdentityCreate(st, tx)
	return err
}

func (IdentityCreate) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	body, identityUrl, sponsor, err := checkIdentityCreate(st, tx)
	if err != nil {
		return nil, err
	}

	if adi, ok := sponsor.(*state.AdiState); ok {
		//this should be done at a higher level...
		if !adi.VerifyKey(tx.Signature[0].PublicKey) {
			return nil, fmt.Errorf("key is not supported by current ADI state")
		}

		if !adi.VerifyAndUpdateNonce(tx.SigInfo.Unused2) {
			return nil, fmt.Errorf("invalid nonce, adi state %d but provided %d", adi.Nonce, tx.SigInfo.Unused2)
		}
	}

	sigSpecUrl := identityUrl.JoinPath("sigspec0")
	ssgUrl := identityUrl.JoinPath("ssg0")

	ss := new(protocol.KeySpec)
	ss.HashAlgorithm = protocol.SHA256
	ss.KeyAlgorithm = protocol.ED25519
	ss.PublicKey = body.PublicKeyHash[:]

	mss := protocol.NewSigSpec()
	mss.ChainUrl = types.String(sigSpecUrl.String()) // TODO Allow override
	mss.Keys = append(mss.Keys, ss)

	ssg := protocol.NewSigSpecGroup()
	ssg.ChainUrl = types.String(ssgUrl.String()) // TODO Allow override
	ssg.SigSpecs = append(ssg.SigSpecs, types.Bytes(sigSpecUrl.ResourceChain()).AsBytes32())

	adi := state.NewADI(types.String(identityUrl.String()), state.KeyTypeSha256, body.PublicKeyHash[:])
	adi.SigSpecId = types.Bytes(ssgUrl.ResourceChain()).AsBytes32()

	scc := new(protocol.SyntheticCreateChain)
	scc.Cause = types.Bytes(tx.TransactionHash()).AsBytes32()
	err = scc.Add(adi, ssg, mss)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal synthetic TX: %v", err)
	}

	syn := new(transactions.GenTransaction)
	syn.SigInfo = &transactions.SignatureInfo{}
	syn.SigInfo.URL = identityUrl.String()
	syn.Transaction, err = scc.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal synthetic TX: %v", err)
	}

	res := new(DeliverTxResult)
	res.AddSyntheticTransaction(syn)
	return res, nil
}
