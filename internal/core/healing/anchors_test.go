// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package healing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Which protocol an anchor is healed with.
//
// Version 2 applies only to a Vandenberg network healing a DN->BVN anchor.
// Everything else takes the version 1 path. Picking wrong means the anchor is
// never healed, and the destination waits forever on a signature threshold it
// will not reach — and nothing exercised this decision at all.

func statusV(v protocol.ExecutorVersion) *api.NetworkStatus {
	return &api.NetworkStatus{ExecutorVersion: v}
}

func TestUseAnchorHealingV2_VandenbergDnToBvn(t *testing.T) {
	s := statusV(protocol.ExecutorVersionV2Vandenberg)
	assert.True(t, useAnchorHealingV2(s, protocol.Directory, "BVN1"),
		"a Vandenberg DN->BVN anchor uses version 2")
}

// Direction matters: BVN->DN keeps the version 1 path even on Vandenberg.
func TestUseAnchorHealingV2_DirectionMatters(t *testing.T) {
	s := statusV(protocol.ExecutorVersionV2Vandenberg)
	assert.False(t, useAnchorHealingV2(s, "BVN1", protocol.Directory),
		"BVN->DN is not a version 2 anchor")
	assert.False(t, useAnchorHealingV2(s, "BVN1", "BVN2"),
		"BVN->BVN is not a version 2 anchor")
	assert.False(t, useAnchorHealingV2(s, protocol.Directory, protocol.Directory),
		"DN->DN is not a version 2 anchor")
}

// Version matters: the same DN->BVN anchor is version 1 before Vandenberg.
func TestUseAnchorHealingV2_RequiresVandenberg(t *testing.T) {
	for _, v := range []protocol.ExecutorVersion{
		protocol.ExecutorVersionV1,
		protocol.ExecutorVersionV2,
		protocol.ExecutorVersionV2Baikonur,
	} {
		assert.False(t, useAnchorHealingV2(statusV(v), protocol.Directory, "BVN1"),
			"executor version %v predates Vandenberg", v)
	}
}

// Partition IDs are compared case-insensitively everywhere else in the
// protocol, and healing is fed IDs from several sources — a scan, a config, an
// operator's command line. A case difference must not silently select the
// wrong recovery path.
func TestUseAnchorHealingV2_PartitionIDsAreCaseInsensitive(t *testing.T) {
	s := statusV(protocol.ExecutorVersionV2Vandenberg)
	for _, dn := range []string{"Directory", "directory", "DIRECTORY", "DiReCtOrY"} {
		assert.True(t, useAnchorHealingV2(s, dn, "bvn1"),
			"source %q should be recognised as the Directory", dn)
	}
	for _, dst := range []string{"Directory", "directory", "DIRECTORY"} {
		assert.False(t, useAnchorHealingV2(s, "Directory", dst),
			"destination %q should be recognised as the Directory", dst)
	}
}

// A nil status must not panic and must not guess.
//
// Healing runs against a network scan that may have failed. Defaulting to the
// newest protocol on missing information would send a version 2 heal into a
// network that cannot answer it; defaulting to version 1 at worst fails the
// way it always did.
func TestUseAnchorHealingV2_NilStatusIsSafeAndConservative(t *testing.T) {
	assert.NotPanics(t, func() {
		assert.False(t, useAnchorHealingV2(nil, protocol.Directory, "BVN1"),
			"missing network status must not select the newer protocol")
	})
}

// Empty partition IDs are not the Directory, and must not be treated as it.
func TestUseAnchorHealingV2_EmptyPartitionIDs(t *testing.T) {
	s := statusV(protocol.ExecutorVersionV2Vandenberg)
	assert.False(t, useAnchorHealingV2(s, "", "BVN1"), "an empty source is not the DN")
	assert.True(t, useAnchorHealingV2(s, protocol.Directory, ""),
		"an empty destination is not the DN, so this stays a DN->non-DN anchor")
}
