package chain

import (
	"fmt"


	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type CreateValidator struct{}

func (CreateValidator) Type() types.TxType { return types.TxTypeCreateValidator }

func (CreateValidator) Validate(st *StateManager, tx *transactions.Envelope) error {
	body := new(protocol.CreateValidator)
	err := tx.As(body)
		if err != nil {
			return fmt.Errorf("invalid payload: %v", err)
		}

		//var myUrl *url.URL

	vally := protocol.NewStakingValidator()

	vally.PubKey = body.PubKey
	vally.Amount = body.Amount


	st.Submit(nil, body)
	st.Commit()
	return nil
}
