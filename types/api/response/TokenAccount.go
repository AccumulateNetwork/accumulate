package response

import (
	"math/big"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
)

type TokenAccount struct {
	*api.TokenAccount
	Balance types.Amount `json:"balance" form:"balance" query:"balance"`
	TxCount uint64       `json:"txCount" form:"txCount" query:"txCount"`
}

func NewTokenAccount(account *api.TokenAccount, bal *big.Int, txCount uint64) *TokenAccount {
	acct := &TokenAccount{}
	acct.TokenAccount = account
	acct.Balance.Set(bal)
	acct.TxCount = txCount
	return acct
}

//
//func (t *TokenAccount) MarshalBinary() ([]byte, error) {
//	t.TokenAccount.URL.MarshalBinary()
//	t.TokenAccount.TokenURL.MarshalBinary()
//}
