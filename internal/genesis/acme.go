package genesis

import (
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

var ACME = new(state.Token)

func createAcmeToken() state.Chain {
	ACME.Type = types.ChainTypeToken

	ACME.ChainUrl = types.String(protocol.AcmeUrl().String())
	ACME.Precision = 8
	ACME.Symbol = "ACME"
	return ACME
}
