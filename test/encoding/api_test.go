// Copyright 2024 The Accumulate Authors
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
	"strings"
	"testing"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/stretchr/testify/require"
	"github.com/ulikunitz/xz"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
	"gopkg.in/src-d/go-git.v4/utils/diff"
)

func TestAPIv2Consistency(t *testing.T) {
	t.Skip("Requires the database states to be updated to the new JSON format")

	jsonrpc2.DebugMethodFunc = true

	// Load test data
	var testData struct {
		Network *accumulated.NetworkInit   `json:"network"`
		State   map[string]json.RawMessage `json:"state"`

		Cases []struct {
			Method   string         `json:"method"`
			Request  map[string]any `json:"request"`
			Response map[string]any `json:"response"`
		} `json:"rpcCalls"`
	}
	b, err := os.ReadFile("../testdata/api-v2-consistency.json.xz")
	require.NoError(t, err)
	r, err := xz.NewReader(bytes.NewBuffer(b))
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(r).Decode(&testData))

	// Start the simulator (do not specify a snapshot option)
	sim, err := simulator.New(
		simulator.WithLogger(acctesting.NewTestLogger(t)),
		simulator.WithDatabase(func(partition *protocol.PartitionInfo, node int, logger log.Logger) keyvalue.Beginner {
			mem := memory.New(nil)
			require.NoError(t, json.Unmarshal(testData.State[partition.ID], mem)) //nolint:staticcheck // FIXME
			return mem
		}),
		simulator.WithNetwork(testData.Network),
		simulator.Deterministic,
	)
	require.NoError(t, err)

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

			case "query":
				if !strings.Contains(c.Request["url"].(string), "/anchors#anchor/") {
					break
				}

				// Don't complain if the new implementation adds info
				delete(res["data"].(map[string]any), "state")
				delete(res, "mainChain")
				delete(res, "merkleState")
			}

			// Don't complain if the new implementation adds info
			if r, ok := res["receipt"].(map[string]any); ok {
				delete(r, "majorBlock")
			}

			jsonDeleteEmpty(c.Response)
			jsonDeleteEmpty(res)

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

func jsonDeleteEmpty(v any) bool {
	if v == nil {
		return true
	}

	switch v := v.(type) {
	case string:
		return v == "0000000000000000000000000000000000000000000000000000000000000000"

	case []any:
		for i := len(v) - 1; i >= 0; i-- {
			if jsonDeleteEmpty(v[i]) {
				v = append(v[:i], v[i+1:]...)
			}
		}
		return len(v) == 0

	case map[string]any:
		var empty []string
		for k, v := range v {
			if jsonDeleteEmpty(v) {
				empty = append(empty, k)
			}
		}
		for _, k := range empty {
			delete(v, k)
		}

		// This is a hack but I don't care about compatibility of this specific
		// field
		delete(v, "gotDirectoryReceipt")

		return len(v) == 0
	}

	return false
}
