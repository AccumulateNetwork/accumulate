// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveWellKnownEndpoint(t *testing.T) {
	cases := map[string]string{
		"mainnet":                              "https://mainnet.accumulatenetwork.io/v3",
		"https://mainnet.accumulatenetwork.io": "https://mainnet.accumulatenetwork.io/v3",
	}

	for input, expect := range cases {
		actual := ResolveWellKnownEndpoint(input, "v3")
		require.Equal(t, expect, actual)
	}
}
