// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package sortutil

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterInPlace(t *testing.T) {
	cases := []struct {
		Before []int
		After  []int
	}{
		{[]int{1, 2, 3}, []int{1, 3}},
		{[]int{2, 4}, []int{}},
		{[]int{1, 3}, []int{1, 3}},
		{[]int{1, 2, 4, 3}, []int{1, 3}},
		{[]int{2, 1, 3, 4}, []int{1, 3}},
		{[]int{1, 2, 3, 4}, []int{1, 3}},
		{[]int{4, 3, 2, 1}, []int{1, 3}},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("Case #%d", i), func(t *testing.T) {
			l := FilterInPlace(c.Before, func(v int) bool { return v%2 != 0 })
			require.ElementsMatch(t, c.After, l)
		})
	}
}
