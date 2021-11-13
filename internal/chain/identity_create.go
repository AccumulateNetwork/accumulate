package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type IdentityCreate struct{}

func (IdentityCreate) Type() types.TxType { return types.TxTypeIdentityCreate }

func checkIdentityCreate(st *StateManager, tx *transactions.GenTransaction) (*protocol.IdentityCreate, *url.URL, state.Chain, error) {
	body := new(protocol.IdentityCreate)
	err := tx.As(body)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid payload: %v", err)
	}

	identityUrl, err := url.Parse(body.Url)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid URL: %v", err)
	}

	err = protocol.IsValidAdiUrl(identityUrl)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid URL: %v", err)
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

	var sigSpecUrl, ssgUrl *url.URL
	if body.KeyPageName == "" {
		sigSpecUrl = identityUrl.JoinPath("sigspec0")
	} else {
		sigSpecUrl = identityUrl.JoinPath(body.KeyPageName)
	}
	if body.KeyBookName == "" {
		ssgUrl = identityUrl.JoinPath("ssg0")
	} else {
		ssgUrl = identityUrl.JoinPath(body.KeyBookName)
	}

	keySpec := new(protocol.KeySpec)
	keySpec.PublicKey = body.PublicKey

	sigSpec := protocol.NewSigSpec()
	sigSpec.ChainUrl = types.String(sigSpecUrl.String()) // TODO Allow override
	sigSpec.Keys = append(sigSpec.Keys, keySpec)
	sigSpec.SigSpecId = types.Bytes(ssgUrl.ResourceChain()).AsBytes32()

	group := protocol.NewSigSpecGroup()
	group.ChainUrl = types.String(ssgUrl.String()) // TODO Allow override
	group.SigSpecs = append(group.SigSpecs, types.Bytes(sigSpecUrl.ResourceChain()).AsBytes32())

	identity := state.NewADI(types.String(identityUrl.String()), state.KeyTypeSha256, body.PublicKey)
	identity.SigSpecId = types.Bytes(ssgUrl.ResourceChain()).AsBytes32()

	st.Create(identity, group, sigSpec)
	return nil
}
