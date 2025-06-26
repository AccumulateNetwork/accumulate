// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"testing"
)

// TestDumpHeaders dumps the headers of the original and reconstructed snapshot files
func TestDumpHeaders(t *testing.T) {
	// Define the paths to the original and reconstructed snapshot files
	originalPath := "/home/paul/work/acc1/dn.snap"
	reconstructedPath := "/home/paul/work/acc1/dn.snap.reconstructed"

	// Dump the header of the original snapshot file
	err := dumpFileHeader(originalPath)
	if err != nil {
		t.Fatalf("Failed to dump original header: %v", err)
	}

	// Dump the header of the reconstructed snapshot file
	err = dumpFileHeader(reconstructedPath)
	if err != nil {
		t.Fatalf("Failed to dump reconstructed header: %v", err)
	}
}
