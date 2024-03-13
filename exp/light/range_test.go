// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package light

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRangeSet(t *testing.T) {
	r := rangeSet{{3, 3}, {6, 8}, {11, 12}}

	r.Add(1)
	require.Equal(t, rangeSet{{1, 1}, {3, 3}, {6, 8}, {11, 12}}, r)

	r.Add(2)
	require.Equal(t, rangeSet{{1, 3}, {6, 8}, {11, 12}}, r)

	r.Add(5)
	require.Equal(t, rangeSet{{1, 3}, {5, 8}, {11, 12}}, r)

	r.Add(9)
	require.Equal(t, rangeSet{{1, 3}, {5, 9}, {11, 12}}, r)

	r.Add(13)
	require.Equal(t, rangeSet{{1, 3}, {5, 9}, {11, 13}}, r)

	r.Add(20)
	require.Equal(t, rangeSet{{1, 3}, {5, 9}, {11, 13}, {20, 20}}, r)

	r.Add(10)
	require.Equal(t, rangeSet{{1, 3}, {5, 13}, {20, 20}}, r)

	r.Add(16)
	require.Equal(t, rangeSet{{1, 3}, {5, 13}, {16, 16}, {20, 20}}, r)
}
