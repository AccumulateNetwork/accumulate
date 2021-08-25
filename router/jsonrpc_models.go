package router

// API data structures, need to merge with Tendermint data structures to avoid duplicates

type ADI struct {
	URL           string `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	PublicKeyHash string `json:"publicKeyHash" form:"publicKeyHash" query:"publicKeyHash" validate:"required,hexadecimal"`
}

type Signer struct {
	URL       string `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	PublicKey string `json:"publicKey" form:"publicKey" query:"publicKey" validate:"required,hexadecimal"`
}

type Token struct {
	URL       string `json:"url" form:"url" query:"url" validate:"required"`
	Symbol    string `json:"symbol" form:"symbol" query:"symbol" validate:"required,alphanum"`
	Precision int    `json:"precision" form:"precision" query:"precision" validate:"required,min=0,max=18"`
}

type TokenAccount struct {
	URL      string `json:"url" form:"url" query:"url" validate:"required"`
	TokenURL string `json:"tokenURL" form:"tokenURL" query:"tokenURL" validate:"required,uri"`
}

type TokenAccountWithBalance struct {
	*TokenAccount
	Balance int64 `json:"balance" form:"balance" query:"balance"`
}

type TokenTx struct {
	Hash string           `json:"hash" form:"hash" query:"hash" validate:"required,hexadecimal"`
	From string           `json:"from" form:"from" query:"from" validate:"required"`
	To   []*TokenTxOutput `json:"to" form:"to" query:"to" validate:"required"`
	Meta []byte           `json:"meta" form:"meta" query:"meta" validate:"required"`
}

type TokenTxOutput struct {
	URL    string `json:"url" form:"url" query:"url" validate:"required"`
	Amount int64  `json:"amount" form:"amount" query:"amount" validate:"gt=0"`
}

// API Request Data Structures

type APIRequest struct {
	Tx  *APIRequestTx `json:"tx" form:"tx" query:"tx" validate:"required"`
	Sig string        `json:"sig" form:"sig" query:"sig" validate:"required,hexadecimal"`
}

type APIRequestTx struct {
	Data      interface{} `json:"data" form:"data" query:"data" validate:"required"`
	Signer    *Signer     `json:"signer" form:"signer" query:"signer" validate:"required"`
	Timestamp int64       `json:"timestamp" form:"timestamp" query:"timestamp" validate:"required"`
}

type APIURLRequest struct {
	URL string `json:"url" form:"url" query:"url" validate:"required"`
}
