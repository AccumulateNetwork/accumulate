// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestAPIv2Consistency(t *testing.T) {
	// Load test data
	var testData struct {
		FinalHeight uint64                   `json:"finalHeight"`
		Network     *accumulated.NetworkInit `json:"network"`
		Genesis     map[string][]byte        `json:"genesis"`
		Roots       map[string][]byte        `json:"snapshots"`

		Submissions []struct {
			Block    uint64             `json:"block"`
			Envelope *protocol.Envelope `json:"envelope"`
			Pending  bool               `json:"pending"`
			Produces bool               `json:"produces"`
		} `json:"submissions"`

		Cases []struct {
			Method   string `json:"method"`
			Request  any    `json:"request"`
			Response any    `json:"response"`
		} `json:"rpcCalls"`
	}
	b, err := os.ReadFile("../testdata/api-v2-consistency.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &testData))

	// Start the simulator
	sim, err := simulator.New(
		acctesting.NewTestLogger(t),
		simulator.MemoryDatabase,
		testData.Network,
		simulator.SnapshotMap(testData.Genesis),
	)
	require.NoError(t, err)

	// Resubmit every user transaction
	for _, sub := range testData.Submissions {
		for sim.BlockIndex(protocol.Directory) < sub.Block {
			require.NoError(t, sim.Step())
		}

		deliveries, err := chain.NormalizeEnvelope(sub.Envelope)
		require.NoError(t, err)
		require.Len(t, deliveries, 1)
		_, err = sim.Submit(deliveries[0])
		require.NoError(t, err)
	}

	// Run the simulator until the final height matches
	for sim.BlockIndex(protocol.Directory) < testData.FinalHeight {
		require.NoError(t, sim.Step())
	}

	// Verify the root hashes of each partition
	for part, root := range testData.Roots {
		require.NoError(t, sim.Database(part).View(func(batch *database.Batch) error {
			require.Equal(t, root, batch.BptRoot())
			return nil
		}))
	}

	// Validate RPC calls
	for i, c := range testData.Cases {
		t.Run(fmt.Sprintf("Call/%d", i), func(t *testing.T) {
			var res any
			err := sim.API().RequestAPIv2(context.Background(), c.Method, c.Request, &res)
			require.NoError(t, err)
			require.Equal(t, c.Response, res)
		})
	}
}
