package router

import (
	"encoding/json"

	"github.com/AccumulateNetwork/accumulated/types"
)

// API data structures, need to merge with Tendermint data structures to avoid duplicates

// Chain will define a new chain to be registered. It will be initialized to the default state
// as defined by validator referenced by the ChainType
type Chain struct {
	URL       types.String    `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	ChainType types.ChainType `json:"chainType" form:"chainType" query:"chainType" validate:"required"` //",hexadecimal"`
}

// ADI structure holds the identity name in the URL.  The name can be stored as acc://<name> or simply <name>
// all chain paths following the ADI domain will be ignored
type ADI struct {
	URL           types.String  `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	PublicKeyHash types.Bytes32 `json:"publicKeyHash" form:"publicKeyHash" query:"publicKeyHash" validate:"required"` //",hexadecimal"`
}

type Token struct {
	URL       types.String `json:"url" form:"url" query:"url" validate:"required"`
	Symbol    types.String `json:"symbol" form:"symbol" query:"symbol" validate:"required,alphanum"`
	Precision types.Byte   `json:"precision" form:"precision" query:"precision" validate:"required,min=0,max=18"`
}

type TokenAccount struct {
	URL      types.String `json:"url" form:"url" query:"url" validate:"required"`
	TokenURL types.String `json:"tokenURL" form:"tokenURL" query:"tokenURL" validate:"required,uri"`
}

type TokenAccountWithBalance struct {
	*TokenAccount
	Balance types.Amount `json:"balance" form:"balance" query:"balance"`
}

type TokenTx struct {
	Hash types.Bytes32    `json:"hash" form:"hash" query:"hash" validate:"required"` //,hexadecimal"`
	From types.String     `json:"from" form:"from" query:"from" validate:"required"`
	To   []*TokenTxOutput `json:"to" form:"to" query:"to" validate:"required"`
	Meta json.RawMessage  `json:"meta" form:"meta" query:"meta" validate:"required"`
}

type TokenTxOutput struct {
	URL    types.UrlChain `json:"url" form:"url" query:"url" validate:"required"`
	Amount types.Amount   `json:"amount" form:"amount" query:"amount" validate:"gt=0"`
}
