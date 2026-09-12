// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !debug

package indexing

// debugMismatch reports a retained state receipt that does not reach the entry
// the proof starts at. In a production build this is not an error: the node
// degrades to an entry-rooted proof, as it does whenever it cannot support a
// main-state start.
func debugMismatch(_, _ []byte) error { return nil }
