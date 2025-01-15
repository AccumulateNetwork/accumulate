// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRandHash_GetRandInt64(t *testing.T) {
	var rh RandHash
	answers := []int64{
		0x66687aadf862bd77,
		0x2b32db6c2c0a6235,
		0x12771355e46cd47c,
		0x7e15c0d3ebe314fa,
		0x376da11fe3ab3d0e,
		0x4391a5c79ffdc798,
		0x5d1adcb5797c2eff,
		0x6a9b711ce5d3749e,
		0x4e6e6acef5953a6a,
		0x713587bc89fe4882,
	}
	for i := 0; i < 10; i++ {
		v := rh.GetRandInt64()
		//fmt.Printf(" %x\n",v)
		require.True(t, answers[i] == v)
	}
}
