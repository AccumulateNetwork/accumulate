package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type UpdateManager struct{}

func (UpdateManager) Type() types.TxType { return types.TxTypeUpdateManager }

func (UpdateManager) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body := new(protocol.UpdateManager)
	err := tx.As(body)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}
	if st.Origin.Header().ManagerKeyBook != "" {
		return nil, fmt.Errorf("manager keybook already assigned")
	}
	book := new(protocol.KeyBook)
	bookUrl, err := url.Parse(body.ManagerKeyBook)
	if err != nil {
		return nil, fmt.Errorf("invalid key book url : %s", body.ManagerKeyBook)
	}
	err = st.LoadUrlAs(bookUrl, book)
	if err != nil {
		return nil, fmt.Errorf("invalid key book: %v", err)
	}
	chain := st.Origin
	chain.Header().ManagerKeyBook = book.ChainUrl
	st.Update(chain)
	return nil, nil
}
