package router

// API data structures, need to merge with Tendermint data structures to avoid duplicates

type Identity struct {
	URL           string `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	PublicKeyHash string `json:"publicKeyHash" form:"publicKeyHash" query:"publicKeyHash" validate:"required,hexadecimal"`
}

type SponsorIdentity struct {
	URL       string `json:"url" form:"url" query:"url" validate:"required,alphanum"`
	PublicKey string `json:"publicKey" form:"publicKey" query:"publicKey" validate:"required,hexadecimal"`
}

type Token struct {
	URL       string `json:"url" form:"url" query:"url" validate:"required"`
	Symbol    string `json:"symbol" form:"symbol" query:"symbol" validate:"required,alphanum"`
	Precision int    `json:"precision" form:"precision" query:"precision" validate:"required,min=0,max=18"`
}

type TokenAddress struct {
	URL      string `json:"url" form:"url" query:"url" validate:"required"`
	TokenURL string `json:"tokenURL" form:"tokenURL" query:"tokenURL" validate:"required,uri"`
}

type TokenAddressWithBalance struct {
	*TokenAddress
	Balance int64 `json:"balance" form:"balance" query:"balance"`
}

// Helpers

type SponsorData struct {
	Sponsor   *SponsorIdentity `json:"sponsor" form:"sponsor" query:"sponsor" validate:"required"`
	Timestamp int64            `json:"timestamp" form:"timestamp" query:"timestamp" validate:"required"`
	Sig       int64            `json:"sig" form:"sig" query:"sig" validate:"required,hexadecimal"`
}

// API requests

type CreateIdentityRequest struct {
	Identity *Identity `json:"identity" form:"identity" query:"identity" validate:"required"`
	*SponsorData
}

type CreateTokenRequest struct {
	Token *Token `json:"token" form:"token" query:"token" validate:"required"`
	*SponsorData
}

type CreateTokenAddressRequest struct {
	TokenAddress *TokenAddress `json:"tokenAddress" form:"tokenAddress" query:"tokenAddress" validate:"required"`
	*SponsorData
}
