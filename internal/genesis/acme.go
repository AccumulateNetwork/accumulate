package genesis

import (
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

var ACME = new(state.Token)

func createAcmeToken() state.Chain {
	ACME.Type = types.ChainTypeTokenIssuer

	ACME.ChainUrl = types.String(protocol.AcmeUrl().String())
	ACME.Precision = 8
	ACME.Symbol = "ACME"
	return ACME
}
