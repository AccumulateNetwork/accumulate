// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

// SetAccountStatesBudget sets how many bytes one page of account states
// carries, and returns what puts it back: how a test pages a small store.
func SetAccountStatesBudget(n int) (restore func()) {
	old := accountStatesBudget
	accountStatesBudget = n
	return func() { accountStatesBudget = old }
}
