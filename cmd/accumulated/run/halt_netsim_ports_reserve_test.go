// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFreeDevnetBaseReserves verifies that a block returned by freeDevnetBase
// stays reserved: a second caller, with nothing bound to the ports yet, must
// not be handed the same block (#4383). Within one process the PID seed is
// identical, which is exactly the congruent-PID case between two processes.
func TestFreeDevnetBaseReserves(t *testing.T) {
	const bvns = 1
	span := (bvns + 1) * int(portBVN)

	var bases []int
	for i := 0; i < 4; i++ {
		bases = append(bases, freeDevnetBase(t, bvns))
	}

	for i, a := range bases {
		for _, b := range bases[i+1:] {
			require.Falsef(t, a < b+span && b < a+span, "blocks %d and %d overlap", a, b)
		}
	}
}
