package synthetic

import (
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type IdentityStateCreate struct {
	Header
	state.IdentityState
}

func NewIdentityStateCreate(adi string) *IdentityStateCreate {
	ctas := &IdentityStateCreate{}
	ctas.AdiChainPath = types.String(adi)

	ctas.Type = "AIM-1"
	return ctas
}
