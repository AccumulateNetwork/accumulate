package types

import "math/big"

type TokenAccount struct {
	URL      String `json:"url" form:"url" query:"url" validate:"required"`
	TokenURL String `json:"tokenURL" form:"tokenURL" query:"tokenURL" validate:"required,uri"`
}

type TokenAccountWithBalance struct {
	*TokenAccount
	Balance Amount `json:"balance" form:"balance" query:"balance"`
}

func NewTokenAccount(accountURL UrlChain, issuingTokenURL UrlChain) *TokenAccount {
	tcc := &TokenAccount{String(accountURL), String(issuingTokenURL)}
	return tcc
}

func NewTokenAccountWithBalance(account *TokenAccount, bal *big.Int) *TokenAccountWithBalance {
	tawb := &TokenAccountWithBalance{}
	tawb.TokenAccount = account
	tawb.Balance.Set(bal)
	return tawb
}
