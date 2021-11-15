package genesis

import (
	"github.com/AccumulateNetwork/accumulate/types/state"
)

func BootstrapStates() []state.Chain {
	return []state.Chain{
		createAcmeToken(),
		createFaucet(),
	}
}
