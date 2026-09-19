// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package network

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func key(t *testing.T) ed25519.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return pub
}

// TestMembershipOf is the one place a committee is read (#4366, #4367). Three
// answers, because "no definition yet" is not "not a member", and the two
// callers need opposite defaults for it — the submit path must not refuse on
// a startup race, the anchor path must not sign on one.
func TestMembershipOf(t *testing.T) {
	mine, other := key(t), key(t)

	g := new(GlobalValues)
	g.Network = new(protocol.NetworkDefinition)
	g.Network.AddValidator(mine, "BVN1", true)
	g.Network.AddValidator(mine, "Directory", false)
	g.Network.AddValidator(other, "BVN1", true)

	require.Equal(t, CommitteeMember, g.MembershipOf(mine, "BVN1"))
	require.Equal(t, CommitteeMember, g.MembershipOf(mine, "bvn1"), "partitions compare case-insensitively")
	require.Equal(t, CommitteeOutsider, g.MembershipOf(mine, "Directory"), "listed but inactive")
	require.Equal(t, CommitteeOutsider, g.MembershipOf(mine, "BVN2"), "no entry for the partition")
	require.Equal(t, CommitteeOutsider, g.MembershipOf(key(t), "BVN1"), "not a validator at all")

	// Unknown, and each of its three shapes.
	var nilG *GlobalValues
	require.Equal(t, CommitteeUnknown, nilG.MembershipOf(mine, "BVN1"), "nil globals must not panic")
	require.Equal(t, CommitteeUnknown, new(GlobalValues).MembershipOf(mine, "BVN1"))
	empty := new(GlobalValues)
	empty.Network = new(protocol.NetworkDefinition)
	require.Equal(t, CommitteeUnknown, empty.MembershipOf(mine, "BVN1"))

	// A node with no key has no identity to be in a committee with (#4367),
	// and that is an outsider, not an unknown: it is known of this node.
	require.Equal(t, CommitteeOutsider, g.MembershipOf(nil, "BVN1"))
	require.Equal(t, CommitteeOutsider, nilG.MembershipOf(nil, "BVN1"))
	require.Equal(t, CommitteeOutsider, g.MembershipOf([]byte{1, 2, 3}, "BVN1"))
}

// TestMembershipOf_DoesNotNeedASortedDefinition — ValidatorByKey is a binary
// search over PublicKeyHash, so a definition that is unsorted or whose hashes
// are unset answers "not in the network". Here a false outsider means
// refusing every submission or withholding every anchor, so the read must not
// depend on either.
func TestMembershipOf_DoesNotNeedASortedDefinition(t *testing.T) {
	a, b, mine := key(t), key(t), key(t)

	g := new(GlobalValues)
	g.Network = &protocol.NetworkDefinition{Validators: []*protocol.ValidatorInfo{
		// Appended in arbitrary order, with no PublicKeyHash — what a
		// hand-built or hand-edited definition looks like.
		{PublicKey: a, Partitions: []*protocol.ValidatorPartitionInfo{{ID: "BVN1", Active: true}}},
		{PublicKey: mine, Partitions: []*protocol.ValidatorPartitionInfo{{ID: "BVN1", Active: true}}},
		{PublicKey: b, Partitions: []*protocol.ValidatorPartitionInfo{{ID: "BVN1", Active: true}}},
	}}
	if _, _, ok := g.Network.ValidatorByKey(mine); ok {
		t.Log("the binary search happened to find it; the walk must anyway")
	}
	require.Equal(t, CommitteeMember, g.MembershipOf(mine, "BVN1"))

	// And an entry carrying only the hash still matches.
	hash := sha256.Sum256(mine)
	g2 := new(GlobalValues)
	g2.Network = &protocol.NetworkDefinition{Validators: []*protocol.ValidatorInfo{
		{PublicKeyHash: hash, Partitions: []*protocol.ValidatorPartitionInfo{{ID: "BVN1", Active: true}}},
	}}
	require.Equal(t, CommitteeMember, g2.MembershipOf(mine, "BVN1"))
}
