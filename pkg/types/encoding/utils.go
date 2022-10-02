// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import "time"

func SplitDuration(d time.Duration) (sec, ns uint64) {
	sec = uint64(d.Seconds())
	ns = uint64((d - d.Round(time.Second)).Nanoseconds())
	return sec, ns
}
