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
	Amount int64  `json:"url" form:"url" query:"url" validate:"gt=0"`
}

// Helpers

type SignerData struct {
	Signer    *Signer `json:"signer" form:"signer" query:"signer" validate:"required"`
	Timestamp int64   `json:"timestamp" form:"timestamp" query:"timestamp" validate:"required"`
	Sig       int64   `json:"sig" form:"sig" query:"sig" validate:"required,hexadecimal"`
}

// API requests

type CreateADIRequest struct {
	ADI *ADI `json:"adi" form:"adi" query:"adi" validate:"required"`
	*SignerData
}

type CreateTokenRequest struct {
	Token *Token `json:"token" form:"token" query:"token" validate:"required"`
	*SignerData
}

type CreateTokenAccountRequest struct {
	TokenAccount *TokenAccount `json:"tokenAccount" form:"tokenAccount" query:"tokenAccount" validate:"required"`
	*SignerData
}

type CreateTokenTxRequest struct {
	TokenTx *TokenTx `json:"tokenTx" form:"tokenTx" query:"tokenTx" validate:"required"`
	*SignerData
}
