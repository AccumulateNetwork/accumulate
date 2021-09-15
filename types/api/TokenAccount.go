package api

import (
	"github.com/AccumulateNetwork/accumulated/types"
)

type TokenAccount struct {
	URL      types.String `json:"url" form:"url" query:"url" validate:"required"`
	TokenURL types.String `json:"tokenURL" form:"tokenURL" query:"tokenURL" validate:"required,uri"`
}

func NewTokenAccount(accountURL types.String, issuingTokenURL types.String) *TokenAccount {
	tcc := &TokenAccount{accountURL, issuingTokenURL}
	return tcc
}
