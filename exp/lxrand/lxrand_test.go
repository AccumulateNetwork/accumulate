// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package lxrand

import (
	"fmt"
	"testing"
	"time"

	"github.com/dustin/go-humanize"
)

func TestSequence(t *testing.T) {
	var r Sequence
	start := time.Now()
	calls := 100000
	for i := 0; i < calls; i++ {
		r.Hash()
	}
	fmt.Printf("Time %10s/s\n", humanize.Comma(int64(float64(calls)/time.Since(start).Seconds())))
}
