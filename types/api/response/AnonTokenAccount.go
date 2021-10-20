package response

import (
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
)

type AnonTokenAccount struct {
	*protocol.TokenAccountCreate
	Balance       types.Amount `json:"balance" form:"balance" query:"balance"`
	TxCount       uint64       `json:"txCount" form:"txCount" query:"txCount"`
	Nonce         uint64       `json:"nonce" form:"nonce" query:"nonce"`
	CreditBalance types.Amount `json:"creditBalance" form:"creditBalance" query:"creditBalance"`
}
