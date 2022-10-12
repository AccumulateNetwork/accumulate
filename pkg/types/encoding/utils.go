// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"math/big"
	"time"
)

func SplitDuration(d time.Duration) (sec, ns uint64) {
	sec = uint64(d.Seconds())
	ns = uint64((d - d.Round(time.Second)).Nanoseconds())
	return sec, ns
}

func BytesCopy(v []byte) []byte {
	u := make([]byte, len(v))
	copy(u, v)
	return v
}

func BigintCopy(v *big.Int) *big.Int {
	u := new(big.Int)
	u.Set(v)
	return u
}
