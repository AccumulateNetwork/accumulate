package synthetic

import (
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type TokenAccountStateCreate struct {
	Header
	state.TokenAccountState
}

//create a default account state
func NewTokenAccountStateCreate(issuerid []byte, issuerchain []byte, coinbase *types.TokenIssuance) *TokenAccountStateCreate {
	ctas := &TokenAccountStateCreate{}
	atas := state.NewTokenAccountState(issuerid, issuerchain, coinbase)
	ctas.Set(atas)
	return ctas
}
