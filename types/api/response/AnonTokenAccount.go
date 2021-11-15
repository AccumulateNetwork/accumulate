package response

import (
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
)

type AnonTokenAccount struct {
	*protocol.TokenAccountCreate
	Balance       types.Amount `json:"balance" form:"balance" query:"balance"`
	TxCount       uint64       `json:"txCount" form:"txCount" query:"txCount"`
	Nonce         uint64       `json:"nonce" form:"nonce" query:"nonce"`
	CreditBalance types.Amount `json:"creditBalance" form:"creditBalance" query:"creditBalance"`
}
