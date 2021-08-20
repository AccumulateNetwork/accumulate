package router

import (
	"encoding/json"
	"github.com/AccumulateNetwork/accumulated/types"
)
// API data structures, need to merge with Tendermint data structures to avoid duplicates

type ADI struct {
	URL           types.String `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	PublicKeyHash types.Bytes32 `json:"publicKeyHash" form:"publicKeyHash" query:"publicKeyHash" validate:"required"`//",hexadecimal"`
}

type Signer struct {
	URL       types.String `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	PublicKey types.Bytes32 `json:"publicKey" form:"publicKey" query:"publicKey" validate:"required"`//,hexadecimal"`
}

type Token struct {
	URL       types.String `json:"url" form:"url" query:"url" validate:"required"`
	Symbol    types.String `json:"symbol" form:"symbol" query:"symbol" validate:"required,alphanum"`
	Precision types.Byte         `json:"precision" form:"precision" query:"precision" validate:"required,min=0,max=18"`
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
	Hash types.Bytes32           `json:"hash" form:"hash" query:"hash" validate:"required"`//,hexadecimal"`
	From types.String           `json:"from" form:"from" query:"from" validate:"required"`
	To   []*TokenTxOutput `json:"to" form:"to" query:"to" validate:"required"`
	Meta json.RawMessage        `json:"meta" form:"meta" query:"meta" validate:"required"`
}

type TokenTxOutput struct {
	URL    types.UrlChain `json:"url" form:"url" query:"url" validate:"required"`
	Amount types.Amount  `json:"amount" form:"amount" query:"amount" validate:"gt=0"`
}

// API Request Data Structures

type APIRequestRaw struct {
	Tx *json.RawMessage  `json:"tx" form:"tx" query:"tx" validate:"required"`
	Sig types.Bytes32    `json:"sig" form:"sig" query:"sig" validate:"required"`
}

type APIRequest struct {
	Tx  *APIRequestTx `json:"tx" form:"tx" query:"tx" validate:"required"`
	Sig types.Bytes32        `json:"sig" form:"sig" query:"sig" validate:"required"`//",hexadecimal"`
}

type APIRequestTx struct {
	Data      interface{} `json:"data" form:"data" query:"data" validate:"required"`
	Signer    *Signer     `json:"signer" form:"signer" query:"signer" validate:"required"`
	Timestamp int64       `json:"timestamp" form:"timestamp" query:"timestamp" validate:"required"`
}
