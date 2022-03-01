package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CreateDirectory struct{}

//func (CreateDirectory) Type() types.TxType { return types.TxTypeCreateDirectory }

func (CreateDirectory) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	// *protocol.IdentityCreate, *url.URL, state.Chain
	body, ok := tx.Transaction.Body.(*protocol.CreateDirectory)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateDirectory), tx.Transaction.Body)
	}

	err := protocol.IsValidAdiUrl(body.Url)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	if body.Url.Path == "" {
		return nil, fmt.Errorf("URL %s is a root ADI, the directory should be a sub ADI", body.Url.String())
	}

	switch st.Origin.(type) {
	case *protocol.ADI:
		// OK
	default:
		return nil, fmt.Errorf("account type %d cannot be the origininator of ADIs", st.Origin.GetType())
	}
	identity := protocol.NewADI()

	identity.Url = body.Url

	st.Create(identity)
	return nil, nil
}
