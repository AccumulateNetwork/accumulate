// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// TestCyclopsBptRepair_TargetCount guards against silent edits — the
// canonical target list for Cyclops is exactly 22. If you change it,
// you must also update the incident document and re-confirm the
// change against fresh sweep data.
func TestCyclopsBptRepair_TargetCount(t *testing.T) {
	require.Len(t, cyclopsBvnTargets, 22,
		"Cyclops BPT repair table must be exactly 22 entries (17 chain-registry strips + 5 body drops); any change requires re-running cmd/snap-bpt-stale + cmd/find-dropped against the canonical Cyclops follower DB and updating docs/incidents/2026-05-cyclops-bpt-drift.md")

	// Cyclops is currently the only partition with a repair list. If
	// that ever changes, this assertion must be relaxed.
	require.Equal(t, 1, len(cyclopsBptRepairTargets),
		"only Cyclops should have a repair list")
	require.Equal(t, cyclopsBvnTargets, cyclopsBptRepairTargets["Cyclops"],
		"Cyclops repair list must match the package-level slice")
}

// TestCyclopsBptRepair_TargetsParse validates every entry in the
// canonical target list — URLs parse, chain types are real, chain
// entries are 32-byte hex.
func TestCyclopsBptRepair_TargetsParse(t *testing.T) {
	seen := make(map[string]int, len(cyclopsBvnTargets))

	for i, tgt := range cyclopsBvnTargets {
		// URL parses
		u, err := url.Parse(tgt.URL)
		require.NoErrorf(t, err, "target[%d] URL %q must parse", i, tgt.URL)
		require.NotNilf(t, u, "target[%d] URL %q produced nil", i, tgt.URL)

		// No duplicates
		if dup, ok := seen[tgt.URL]; ok {
			t.Fatalf("target[%d] URL %q duplicates target[%d]", i, tgt.URL, dup)
		}
		seen[tgt.URL] = i

		// Chains: type valid, entries are 32-byte hex
		for j, c := range tgt.Chains {
			require.NotEmptyf(t, c.Name, "target[%d] chain[%d] missing name", i, j)
			require.Truef(t, isValidChainType(c.Type),
				"target[%d] chain[%d] (%s) has invalid type %v", i, j, c.Name, c.Type)
			for k, h := range c.Entries {
				raw, err := hex.DecodeString(h)
				require.NoErrorf(t, err,
					"target[%d] chain[%d] (%s) entry[%d] %q is not valid hex", i, j, c.Name, k, h)
				require.Lenf(t, raw, 32,
					"target[%d] chain[%d] (%s) entry[%d] must decode to 32 bytes, got %d", i, j, c.Name, k, len(raw))
			}
		}
	}
}

// TestCyclopsBptRepair_ClassBreakdown asserts the embedded data
// matches the documented split: 17 with-chains entries (Class A) and
// 5 body-drop entries (Class B, no chains, leaf will recompute to
// empty-state hash).
func TestCyclopsBptRepair_ClassBreakdown(t *testing.T) {
	var classA, classB int
	for _, tgt := range cyclopsBvnTargets {
		if len(tgt.Chains) == 0 {
			classB++
		} else {
			classA++
		}
	}
	require.Equal(t, 17, classA, "expected 17 Class-A (chain-registry) targets")
	require.Equal(t, 5, classB, "expected 5 Class-B (body-drop) targets")
}

func isValidChainType(t merkle.ChainType) bool {
	switch t {
	case merkle.ChainTypeTransaction, merkle.ChainTypeIndex, merkle.ChainTypeAnchor:
		return true
	}
	return false
}
