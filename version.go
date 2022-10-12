// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulate

const unknownVersion = "version unknown"

var Version = unknownVersion
var Commit string

func IsVersionKnown() bool {
	return Version != unknownVersion
}
