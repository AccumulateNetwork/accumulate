// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
	"gopkg.in/src-d/go-git.v4/utils/diff"
)

func TestAPIv2Consistency(t *testing.T) {
	t.Skip("FIXME once 1.1 is done")

	jsonrpc2.DebugMethodFunc = true

	// Load test data
	var testData struct {
		FinalHeight uint64                   `json:"finalHeight"`
		Network     *accumulated.NetworkInit `json:"network"`
		Genesis     map[string][]byte        `json:"genesis"`
		Roots       map[string][]byte        `json:"roots"`

		Submissions []struct {
			Block    uint64              `json:"block"`
			Envelope *messaging.Envelope `json:"envelope"`
			Pending  bool                `json:"pending"`
			Produces bool                `json:"produces"`
		} `json:"submissions"`

		Cases []struct {
			Method   string         `json:"method"`
			Request  map[string]any `json:"request"`
			Response map[string]any `json:"response"`
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
	sim.Deterministic = true

	// Resubmit every user transaction
	for _, sub := range testData.Submissions {
		require.LessOrEqual(t, sim.BlockIndex(protocol.Directory), sub.Block)
		for sim.BlockIndex(protocol.Directory) < sub.Block {
			require.NoError(t, sim.Step())
		}

		messages, err := sub.Envelope.Normalize()
		require.NoError(t, err)
		_, err = sim.Submit(messages)
		require.NoError(t, err)
	}

	// Run the simulator until the final height matches
	require.LessOrEqual(t, sim.BlockIndex(protocol.Directory), testData.FinalHeight)
	for sim.BlockIndex(protocol.Directory) < testData.FinalHeight {
		require.NoError(t, sim.Step())
	}

	// Verify the root hashes of each partition
	for part, root := range testData.Roots {
		View(t, sim.Database(part), func(batch *database.Batch) {
			require.Equal(t, root, batch.BptRoot())
		})
	}

	// Validate RPC calls
	for i, c := range testData.Cases {
		t.Run(fmt.Sprintf("Call/%d", i), func(t *testing.T) {
			var res map[string]any
			err := sim.ClientV2(protocol.Directory).RequestAPIv2(context.Background(), c.Method, c.Request, &res)
			if err != nil {
				t.Logf("Case %d", i)
				req, _ := json.Marshal(c.Request)
				t.Log(c.Method, string(req))
				require.NoError(t, err)
			}

			// Patch the results due to weird behavior of OG API v2
			switch c.Method {
			case "query":
				switch {
				case res["transaction"] != nil,
					res["type"] == "chainEntry":
					// OG API v2 adds empty values for these fields for some reason
					if _, ok := res["mainChain"]; !ok {
						res["mainChain"] = map[string]any{}
						res["merkleState"] = map[string]any{}
					}
				}

			case "query-major-blocks":
				// OG API v2 query-major-blocks is buggy and reports too few minor
				// blocks
				major := res["items"].([]any)[0].(map[string]any)
				minor := major["minorBlocks"].([]any)

				// Remove minor block entries 29 and on
				i, _ := sortutil.Search(minor, func(v any) int {
					return int(v.(map[string]any)["blockIndex"].(float64)) - 29
				})
				major["minorBlocks"] = minor[:i]
			}

			// Don't complain if the new implementation adds info
			if r, ok := res["receipt"].(map[string]any); ok {
				delete(r, "majorBlock")
			}

			expect, _ := json.MarshalIndent(c.Response, "", "  ")
			actual, _ := json.MarshalIndent(res, "", "  ")
			if !bytes.Equal(expect, actual) {
				t.Logf("Case %d", i)
				req, _ := json.Marshal(c.Request)
				t.Log(c.Method, string(req))
				d := diff.Do(string(expect), string(actual))
				t.Fatal(diffmatchpatch.New().DiffPrettyText(d))
			}
		})
	}
}
