// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package routing

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestBuildRoutingTable(t *testing.T) {
	cases := []struct {
		Count  int
		Expect []protocol.Route
	}{
		{2, []protocol.Route{
			{Length: 1, Value: 0b0, Partition: "A"},
			{Length: 1, Value: 0b1, Partition: "B"},
		}},
		{3, []protocol.Route{
			{Length: 1, Value: 0b0, Partition: "A"},
			{Length: 2, Value: 0b10, Partition: "B"},
			{Length: 2, Value: 0b11, Partition: "C"},
		}},
		{4, []protocol.Route{
			{Length: 2, Value: 0b00, Partition: "A"},
			{Length: 2, Value: 0b01, Partition: "B"},
			{Length: 2, Value: 0b10, Partition: "C"},
			{Length: 2, Value: 0b11, Partition: "D"},
		}},
		{5, []protocol.Route{
			{Length: 2, Value: 0b00, Partition: "A"},
			{Length: 2, Value: 0b01, Partition: "B"},
			{Length: 2, Value: 0b10, Partition: "C"},
			{Length: 3, Value: 0b110, Partition: "D"},
			{Length: 3, Value: 0b111, Partition: "E"},
		}},
	}

	for _, c := range cases {
		t.Run(fmt.Sprint(c.Count), func(t *testing.T) {
			bvns := make([]string, c.Count)
			for i := range bvns {
				bvns[i] = string(rune('A' + i))
			}
			routes := buildSimpleTable(bvns, 0, 0)
			require.Equal(t, len(c.Expect), len(routes))
			for i, expect := range c.Expect {
				assert.Equal(t, expect, routes[i])
			}
		})
	}
}
