package types

import "encoding/json"

// API Request Support Structure

// Signer holds the ADI and public key to use to verify the transaction
type Signer struct {
	URL       String  `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	PublicKey Bytes32 `json:"publicKey" form:"publicKey" query:"publicKey" validate:"required"`
}

// API Request Data Structures

type APIRequestRaw struct {
	Tx  *json.RawMessage `json:"tx" form:"tx" query:"tx" validate:"required"`
	Sig Bytes32          `json:"sig" form:"sig" query:"sig" validate:"required"`
}

type APIRequest struct {
	Tx  *APIRequestTx `json:"tx" form:"tx" query:"tx" validate:"required"`
	Sig Bytes32       `json:"sig" form:"sig" query:"sig" validate:"required"` //",hexadecimal"`
}

type APIRequestTx struct {
	Data      interface{} `json:"data" form:"data" query:"data" validate:"required"`
	Signer    *Signer     `json:"signer" form:"signer" query:"signer" validate:"required"`
	Timestamp int64       `json:"timestamp" form:"timestamp" query:"timestamp" validate:"required"`
}
