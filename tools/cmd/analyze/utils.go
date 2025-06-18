// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"fmt"
	"sort"
)

// printSortedStats prints statistics in sorted order
func printSortedStats(stats map[string]int) {
	// Sort keys for consistent output
	keys := make([]string, 0, len(stats))
	for k := range stats {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Print stats
	for _, k := range keys {
		fmt.Printf("  %s: %d\n", k, stats[k])
	}
}

// writeSortedStats writes statistics in sorted order to a writer
func writeSortedStats(writer *bufio.Writer, stats map[string]int) {
	// Sort keys for consistent output
	keys := make([]string, 0, len(stats))
	for k := range stats {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Write stats
	for _, k := range keys {
		fmt.Fprintf(writer, "  %s: %d\n", k, stats[k])
	}
}
