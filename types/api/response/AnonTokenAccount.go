package response

import (
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

type LiteTokenAccount struct {
	*protocol.CreateTokenAccount
	Balance       types.Amount `json:"balance" form:"balance" query:"balance"`
	Nonce         uint64       `json:"nonce" form:"nonce" query:"nonce"`
	CreditBalance types.Amount `json:"creditBalance" form:"creditBalance" query:"creditBalance"`
}
