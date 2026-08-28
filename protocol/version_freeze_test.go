// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #4139: the two development lines assigned different numbers to the same
// version names, and nothing failed — versions were compared and gated but
// their literal values were never asserted anywhere. These tests freeze the
// table so any future renumber is a deliberate edit to a test, not a silent
// divergence discovered at activation time.

// The important one: the literal numbers. Mainnet sits at v2-vandenberg (7);
// everything above it is unclaimed on the one network that cannot be rebuilt,
// and this table is the claim.
func TestExecutorVersionNumbersAreFrozen(t *testing.T) {
	frozen := map[ExecutorVersion]uint64{
		ExecutorVersionV1:                   1,
		ExecutorVersionV1SignatureAnchoring: 2,
		ExecutorVersionV1DoubleHashEntries:  3,
		ExecutorVersionV1Halt:               4,
		ExecutorVersionV2:                   5,
		ExecutorVersionV2Baikonur:           6,
		ExecutorVersionV2Vandenberg:         7,
		ExecutorVersionV2Jiuquan:            8,
		ExecutorVersionV2Tanegashima:        9,
		ExecutorVersionV2Kourou:             10,
		ExecutorVersionVNext:                11,
	}
	for v, n := range frozen {
		assert.Equal(t, n, uint64(v),
			"%v must be %d — renumbering an executor version is a network migration, not an edit", v, n)
	}
}

func TestExecutorVersionsAreOrdered(t *testing.T) {
	ladder := []ExecutorVersion{
		ExecutorVersionV1,
		ExecutorVersionV1SignatureAnchoring,
		ExecutorVersionV1DoubleHashEntries,
		ExecutorVersionV1Halt,
		ExecutorVersionV2,
		ExecutorVersionV2Baikonur,
		ExecutorVersionV2Vandenberg,
		ExecutorVersionV2Jiuquan,
		ExecutorVersionV2Tanegashima,
		ExecutorVersionV2Kourou,
		ExecutorVersionVNext,
	}
	for i := 1; i < len(ladder); i++ {
		assert.Less(t, ladder[i-1], ladder[i],
			"versions are ordinal — every activation climbs the ladder one rung at a time")
	}
}

// The init() panic in version.go guards the SHAPE (Latest+1 == VNext); this
// guards the VALUE.
func TestExecutorVersionLatestIsKourou(t *testing.T) {
	assert.Equal(t, ExecutorVersionV2Kourou, ExecutorVersionLatest)
}

// A renumber cannot silently move a label onto a different number: every
// version's string maps back to the same value.
func TestVersionLabelRoundTrip(t *testing.T) {
	for v := ExecutorVersionV1; v <= ExecutorVersionVNext; v++ {
		name := v.String()
		require.NotContains(t, name, "ExecutorVersion:",
			"version %d has no label — a version without a name cannot be activated", uint64(v))
		got, ok := ExecutorVersionByName(name)
		require.True(t, ok, "label %q must resolve", name)
		assert.Equal(t, v, got, "label %q must round-trip to the same number", name)
	}
}

func TestV2TanegashimaEnabled_Boundaries(t *testing.T) {
	assert.False(t, ExecutorVersionV2Jiuquan.V2TanegashimaEnabled(), "false at 8")
	assert.True(t, ExecutorVersionV2Tanegashima.V2TanegashimaEnabled(), "true at 9")
	assert.True(t, ExecutorVersionV2Kourou.V2TanegashimaEnabled(), "true above 9")
	assert.True(t, ExecutorVersionVNext.V2TanegashimaEnabled())
}

func TestV2KourouEnabled_Boundaries(t *testing.T) {
	assert.False(t, ExecutorVersionV2Tanegashima.V2KourouEnabled(), "false at 9")
	assert.True(t, ExecutorVersionV2Kourou.V2KourouEnabled(), "true at 10")
	assert.True(t, ExecutorVersionVNext.V2KourouEnabled(), "true above 10")
}

func TestUnknownVersionStringIsRejected(t *testing.T) {
	for _, name := range []string{"", "v3", "kourou", "v2-kourou-2", "latest"} {
		_, ok := ExecutorVersionByName(name)
		assert.False(t, ok, "%q must be rejected, not resolved to a silent zero", name)
	}
}
