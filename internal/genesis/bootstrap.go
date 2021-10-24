package genesis

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

func BootstrapStates() (map[types.Bytes32]*state.Object, error) {
	m := make(map[types.Bytes32]*state.Object)

	c, o := createAcmeToken()
	if o == nil || c == nil {
		return nil, fmt.Errorf("genesis cannot create ACME token")
	}
	m[*c] = o

	c, o = createFaucet()
	if o == nil || c == nil {
		return nil, fmt.Errorf("genesis cannot create faucet")
	}
	m[*c] = o

	return m, nil
}
