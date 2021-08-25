package types

type TokenChainCreate struct {
	IssuingAdiChainPath String `json:"issuing-adi-chain-path"`
}

func NewTokenChainCreate(issuingadicp string) *TokenChainCreate {
	tcc := &TokenChainCreate{String(issuingadicp)}
	return tcc
}
