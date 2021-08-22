package synthetic

import (
	"crypto/sha256"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type AdiStateCreate struct {
	Header
	state.AdiState
}

func NewIdentityStateCreate(adi string) *AdiStateCreate {
	ctas := &AdiStateCreate{}
	ctas.AdiChainPath = types.String(adi)

	ctas.Type = sha256.Sum256([]byte("AIM/0/0.1"))
	return ctas
}
