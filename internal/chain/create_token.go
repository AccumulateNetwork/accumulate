package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type CreateToken struct{}

func (CreateToken) Type() types.TxType { return types.TxTypeCreateToken }

func (CreateToken) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body := new(protocol.CreateToken)
	err := tx.As(body)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	tokenUrl, err := url.Parse(body.Url)
	if err != nil {
		return nil, fmt.Errorf("invalid token URL: %v", err)
	}

	if body.KeyBookUrl == "" {
		body.KeyBookUrl = string(st.Origin.Header().KeyBook)
	} else {
		u, err := url.Parse(body.KeyBookUrl)
		if err != nil {
			return nil, fmt.Errorf("invalid key book URL: %v", err)
		}
		body.KeyBookUrl = u.String()
	}

	if body.Precision > 18 {
		return nil, fmt.Errorf("precision must be in range 0 to 18")
	}

	token := protocol.NewTokenIssuer()
	token.ChainUrl = types.String(tokenUrl.String())
	token.KeyBook = types.String(body.KeyBookUrl)
	token.Precision = body.Precision
	token.Supply = body.InitialSupply
	token.HasSupplyLimit = body.HasSupplyLimit
	token.Symbol = body.Symbol
	if body.Properties != "" {
		token.Properties = body.Properties
	}

	st.Create(token)
	return nil, nil
}
