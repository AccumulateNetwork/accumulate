// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build dagbft

package run

func (p partOpts) addSnapshotService(cfg *Config) {
	// Snapshots are not supported in dagbft mode
}
