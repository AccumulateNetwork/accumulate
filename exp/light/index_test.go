// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package light

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestIndex(t *testing.T) {
	entries := []*protocol.IndexEntry{
		{Source: 4},
		{Source: 8},
	}

	cases := []struct {
		Source uint64
		Result int
	}{
		{0, 0},
		{2, 0},
		{4, 0},
		{6, 1},
		{8, 1},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			i, _, ok := FindEntry(entries, ByIndexSource(c.Source))
			require.True(t, ok)
			require.Equal(t, c.Result, i)
		})
	}
}
