package router

import (
	"encoding/json"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
)

// API data structures, need to merge with Tendermint data structures to avoid duplicates

// Chain will define a new chain to be registered. It will be initialized to the default state
// as defined by validator referenced by the ChainType
type Chain struct {
	URL       types.String  `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	ChainType api.ChainType `json:"chainType" form:"chainType" query:"chainType" validate:"required"` //",hexadecimal"`
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

// API Request Support Structure

// Signer holds the ADI and public key to use to verify the transaction
type Signer struct {
	URL       types.String  `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	PublicKey types.Bytes32 `json:"publicKey" form:"publicKey" query:"publicKey" validate:"required"`
}

// API Request Data Structures

type APIRequestRaw struct {
	Tx  *json.RawMessage `json:"tx" form:"tx" query:"tx" validate:"required"`
	Sig types.Bytes64    `json:"sig" form:"sig" query:"sig" validate:"required"`
}

type APIRequest struct {
	Tx  *APIRequestTx `json:"tx" form:"tx" query:"tx" validate:"required"`
	Sig types.Bytes64 `json:"sig" form:"sig" query:"sig" validate:"required"` //",hexadecimal"`
}

type APIRequestTx struct {
	Data      interface{} `json:"data" form:"data" query:"data" validate:"required"`
	Signer    *Signer     `json:"signer" form:"signer" query:"signer" validate:"required"`
	Timestamp int64       `json:"timestamp" form:"timestamp" query:"timestamp" validate:"required"`
}
