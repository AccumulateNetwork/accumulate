package api

import (
	"github.com/AccumulateNetwork/accumulated/types"
	"math/big"
)

type TokenAccount struct {
	URL      types.String `json:"url" form:"url" query:"url" validate:"required"`
	TokenURL types.String `json:"tokenURL" form:"tokenURL" query:"tokenURL" validate:"required,uri"`
}

type TokenAccountWithBalance struct {
	*TokenAccount
	Balance types.Amount `json:"balance" form:"balance" query:"balance"`
}

func NewTokenAccount(accountURL types.UrlChain, issuingTokenURL types.UrlChain) *TokenAccount {
	tcc := &TokenAccount{types.String(accountURL), types.String(issuingTokenURL)}
	return tcc
}

func NewTokenAccountWithBalance(account *TokenAccount, bal *big.Int) *TokenAccountWithBalance {
	tawb := &TokenAccountWithBalance{}
	tawb.TokenAccount = account
	tawb.Balance.Set(bal)
	return tawb
}
