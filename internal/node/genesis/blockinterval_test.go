// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package genesis_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4267: a network is deployed with a stated cadence, and the value is written
// into genesis so it can be read back from the network. A wrong value is
// refused at deployment rather than substituted.
func TestGenesisRefusesANegativeBlockInterval(t *testing.T) {
	err := genesis.Init(new(ioutil2.Buffer), genesis.InitOpts{
		NetworkID:   "test",
		PartitionId: protocol.Directory,
		NetworkType: protocol.PartitionTypeDirectory,
		GenesisTime: time.Now(),
		GenesisGlobals: &network.GlobalValues{
			Globals: &protocol.NetworkGlobals{BlockInterval: -1 * time.Second},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "block interval")
}
