package genesis

import (
	"github.com/AccumulateNetwork/accumulated/types/state"
)

func BootstrapStates() []state.Chain {
	return []state.Chain{
		createAcmeToken(),
		createFaucet(),
	}
}
