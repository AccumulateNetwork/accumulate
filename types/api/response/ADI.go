package response

import (
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type ADI struct {
	protocol.IdentityCreate
	state.AdiState
}
