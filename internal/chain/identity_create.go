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

func checkIdentityCreate(st *StateManager, tx *transactions.GenTransaction) (*api.ADI, *url.URL, state.Chain, error) {
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
	switch st.Sponsor.(type) {
	case *protocol.AnonTokenAccount, *state.AdiState:
		// OK
	default:
		return nil, nil, nil, fmt.Errorf("chain type %d cannot sponsor ADIs", st.Sponsor.Header().Type)
	}

	return body, identityUrl, sponsor, nil
}

func (IdentityCreate) CheckTx(st *StateManager, tx *transactions.GenTransaction) error {
	_, _, _, err := checkIdentityCreate(st, tx)
	return err
}

func (IdentityCreate) DeliverTx(st *StateManager, tx *transactions.GenTransaction) error {
	body, identityUrl, _, err := checkIdentityCreate(st, tx)
	if err != nil {
		return err
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
		return fmt.Errorf("failed to marshal synthetic TX: %v", err)
	}

	st.Submit(identityUrl, scc)
	return nil
}
