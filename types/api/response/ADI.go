package response

import (
	"github.com/AccumulateNetwork/accumulated/protocol"
)

type ADI struct {
	protocol.IdentityCreate `json:"token"`
}
