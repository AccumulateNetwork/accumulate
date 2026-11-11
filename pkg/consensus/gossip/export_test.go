// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package gossip

// SetBeforeCloseRequest runs fn in each fetch between writing the request and
// half-closing the stream, until the returned func restores the default.
func SetBeforeCloseRequest(fn func()) (restore func()) {
	old := beforeCloseRequest
	beforeCloseRequest = fn
	return func() { beforeCloseRequest = old }
}
