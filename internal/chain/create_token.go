package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type CreateToken struct{}

func (CreateToken) Type() types.TxType { return types.TxTypeCreateToken }

func (CreateToken) Validate(st *StateManager, tx *transactions.Envelope) error {
	body := new(protocol.CreateToken)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	tokenUrl, err := url.Parse(body.Url)
	if err != nil {
		return fmt.Errorf("invalid token URL: %v", err)
	}

	if body.KeyBookUrl == "" {
		body.KeyBookUrl = string(st.Origin.Header().KeyBook)
	} else {
		u, err := url.Parse(body.KeyBookUrl)
		if err != nil {
			return fmt.Errorf("invalid key book URL: %v", err)
		}
		body.KeyBookUrl = u.String()
	}

	if body.Precision > 18 {
		return fmt.Errorf("precision must be in range 0 to 18")
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
	return nil
}
