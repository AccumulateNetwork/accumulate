// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package spec_test

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	require.NoError(t, err)
	return string(b)
}

func TestDifferencesHeadingsAreUnique(t *testing.T) {
	headings := regexp.MustCompile(`(?m)^### (C\d+)\b`).FindAllStringSubmatch(read(t, "DIFFERENCES.md"), -1)
	seen := map[string]bool{}
	for _, h := range headings {
		require.Falsef(t, seen[h[1]], "DIFFERENCES.md has two headings for %s", h[1])
		seen[h[1]] = true
	}
}

func TestConsensusCitesDifferences(t *testing.T) {
	differences := read(t, "DIFFERENCES.md")
	cited := regexp.MustCompile(`\bC\d+\b`).FindAllString(read(t, "consensus.md"), -1)
	require.NotEmpty(t, cited)
	for _, c := range cited {
		n := len(regexp.MustCompile(`(?m)^### `+c+`\b`).FindAllString(differences, -1))
		require.Equalf(t, 1, n, "consensus.md cites %s; DIFFERENCES.md has %d headings for it", c, n)
	}
}

func TestConsensusIsWritten(t *testing.T) {
	require.NotContains(t, read(t, "consensus.md"), "still to be written")
}
