// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types

import "encoding/hex"

// hexEncode returns the hex-encoded string of the given bytes.
func hexEncode(b []byte) string {
	return hex.EncodeToString(b)
}
