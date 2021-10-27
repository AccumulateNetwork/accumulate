package genesis

import (
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

func createAcmeToken() (*types.Bytes32, *state.Object) {
	token := state.Token{}
	token.Type = types.ChainTypeToken

	token.ChainUrl = types.String(protocol.AcmeUrl().String())
	token.Precision = 8
	token.Symbol = "ACME"
	//desc := json.RawMessage("{\"propertiesUrl\":\"acc://acme/properties"}")
	//propertiesUrl would contain attributes associated with the token that can be updated.
	t, err := token.MarshalBinary()
	if err != nil {
		return nil, nil
	}

	o := &state.Object{}
	o.Entry = t
	chainId := types.Bytes(protocol.AcmeUrl().ResourceChain()).AsBytes32()
	return &chainId, o
}
