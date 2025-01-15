// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import "math"

func (r *Rational) Set(num, denom uint64) {
	r.Numerator, r.Denominator = num, denom
}

// Threshold returns keyCount * num / denom rounded up.
func (r *Rational) Threshold(keyCount int) uint64 {
	v := float64(keyCount) * float64(r.Numerator) / float64(r.Denominator)
	return uint64(math.Ceil(v))
}
