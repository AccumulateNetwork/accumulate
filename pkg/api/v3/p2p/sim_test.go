// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p_test

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

func TestSim(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Initialize
	sim := NewSim(t,
		simulator.LocalNetwork(t.Name(), 3, 3, net.ParseIP("127.0.1.1"), 12345),
		simulator.Genesis(GenesisTime),
	)

	err := sim.S.ListenAndServe(ctx, simulator.ListenOptions{
		ListenP2Pv3: true,
		ServeError:  func(err error) { require.NoError(t, err) },
	})
	require.NoError(t, err)
}
