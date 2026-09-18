// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import "testing"

// TestBptPageCount — a count past the cap is answered with the cap. It used to
// fall back to the default, so the 4096 cap was unreachable and a client that
// asked for more than 4096 got 256. A simulator's tree is smaller than either,
// so only the rule itself can say which happened.
func TestBptPageCount(t *testing.T) {
	cases := []struct {
		asked uint64
		want  int
	}{
		{0, defaultBptPageSize},
		{1, 1},
		{255, 255},
		{maxBptPageSize, maxBptPageSize},
		{maxBptPageSize + 1, maxBptPageSize},
		{1 << 20, maxBptPageSize},
	}
	for _, c := range cases {
		if got := bptPageCount(c.asked); got != c.want {
			t.Errorf("bptPageCount(%d) = %d, want %d", c.asked, got, c.want)
		}
	}
}
