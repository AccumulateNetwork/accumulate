package synthetic

import (
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type AdiStateCreate struct {
	Header
	state.AdiState
}

func NewIdentityStateCreate(adi string) *AdiStateCreate {
	ctas := &AdiStateCreate{}
	ctas.SetHeader(types.UrlChain(adi), api.ChainTypeAdi[:])
	return ctas
}
