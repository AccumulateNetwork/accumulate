package response

import (
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"math/big"
)

type TokenAccount struct {
	*api.TokenAccount
	Balance types.Amount `json:"balance" form:"balance" query:"balance"`
}

func NewTokenAccount(account *api.TokenAccount, bal *big.Int) *TokenAccount {
	acct := &TokenAccount{}
	acct.TokenAccount = account
	acct.Balance.Set(bal)
	return acct
}

//
//func (t *TokenAccount) MarshalBinary() ([]byte, error) {
//	t.TokenAccount.URL.MarshalBinary()
//	t.TokenAccount.TokenURL.MarshalBinary()
//}
