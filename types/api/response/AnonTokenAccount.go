package response

import (
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
)

type AnonTokenAccount struct {
	*api.TokenAccount
	Balance       types.Amount `json:"balance" form:"balance" query:"balance"`
	TxCount       uint64       `json:"txCount" form:"txCount" query:"txCount"`
	Nonce         uint64       `json:"nonce" form:"nonce" query:"nonce"`
	CreditBalance types.Amount `json:"creditBalance" form:"creditBalance" query:"creditBalance"`
}
