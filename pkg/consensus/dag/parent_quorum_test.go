// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dag_test

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// TestDAG_InsertRequiresParentQuorum: a certificate above round 0 enters the
// DAG only with distinct parents of the round before it that carry a quorum
// (Narwhal 4.1, #4591). An empty parent list used to skip the parent check
// altogether, and the parents' round and quorum were never checked, so a
// leader certificate with no path to an earlier leader could be committed
// and two nodes that received the same certificates in a different order
// committed different sequences.
func TestDAG_InsertRequiresParentQuorum(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d := dag.NewDAG(10)
	var genesis []types.CertificateDigest
	for i := 0; i < 4; i++ {
		cert := createTestCert(t, committee, privKeys, i, 0, nil)
		require.NoError(t, d.InsertGenesis(cert))
		genesis = append(genesis, cert.Digest())
	}

	// Without a stake rule installed, one parent of the round before is
	// enough; none is not.
	require.Error(t, d.Insert(createTestCert(t, committee, privKeys, 0, 1, nil)),
		"a round-1 certificate with no parents must not insert")
	one := createTestCert(t, committee, privKeys, 0, 1, genesis[:1])
	require.NoError(t, d.Insert(one))

	// With the committee's stake rule, the parents of the round before must
	// carry a quorum: three of four here.
	var asked types.Round
	d.SetParentQuorum(func(round types.Round, authors []ed25519.PublicKey) bool {
		asked = round
		var stake uint64
		for _, a := range authors {
			stake += committee.StakeOf(a)
		}
		return committee.HasQuorum(stake)
	})
	two := createTestCert(t, committee, privKeys, 1, 1, genesis[:2])
	require.ErrorIs(t, d.Insert(two), dag.ErrParentQuorum)
	// A parent named twice is refused outright, not counted once: the
	// voter has no certificate to verify, and Certificate.Verify refuses
	// the repeat, so a vote for such a header is for a certificate that
	// can never exist. The error is not the quorum one and not "parent
	// certificate not found", so the caller drops the certificate instead
	// of holding it.
	twice := createTestCert(t, committee, privKeys, 1, 1, []types.CertificateDigest{genesis[0], genesis[1], genesis[1]})
	err := d.Insert(twice)
	require.Error(t, err)
	require.NotErrorIs(t, err, dag.ErrParentQuorum)
	require.Contains(t, err.Error(), "twice")
	require.Contains(t, err.Error(), genesis[1].String(), "the error names the repeated parent")
	// A full quorum with one repeat is still refused: the repeat alone is
	// the reason.
	repeat := createTestCert(t, committee, privKeys, 3, 1, []types.CertificateDigest{genesis[0], genesis[1], genesis[2], genesis[0]})
	err = d.Insert(repeat)
	require.Error(t, err)
	require.NotErrorIs(t, err, dag.ErrParentQuorum)
	require.Contains(t, err.Error(), "twice")
	three := createTestCert(t, committee, privKeys, 1, 1, genesis[:3])
	require.NoError(t, d.Insert(three))
	require.Equal(t, types.Round(0), asked, "the quorum is of the parents' round")
	four := createTestCert(t, committee, privKeys, 2, 1, genesis)
	require.NoError(t, d.Insert(four))

	// A weak link to an older round is allowed and not counted; a parent at
	// the certificate's own round or above is refused.
	round1 := []types.CertificateDigest{one.Digest(), three.Digest(), four.Digest()}
	weak := createTestCert(t, committee, privKeys, 0, 2, append([]types.CertificateDigest{genesis[3]}, round1...))
	require.NoError(t, d.Insert(weak))
	require.Equal(t, types.Round(1), asked)
	onlyWeak := createTestCert(t, committee, privKeys, 1, 2, genesis)
	require.ErrorIs(t, d.Insert(onlyWeak), dag.ErrParentQuorum)
	same := createTestCert(t, committee, privKeys, 1, 2, append([]types.CertificateDigest{weak.Digest()}, round1...))
	require.Error(t, d.Insert(same), "a parent at the certificate's own round must be refused")

	// A certificate still missing a parent is reported as such, so the
	// caller holds it and asks for the parent.
	missing := createTestCert(t, committee, privKeys, 1, 2, append([]types.CertificateDigest{{9, 9, 9}}, round1...))
	err = d.Insert(missing)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parent certificate not found")

	// CheckParents is the same rule for a header a voter is asked to sign.
	require.NoError(t, d.CheckParents(2, round1))
	require.ErrorIs(t, d.CheckParents(2, round1[:2]), dag.ErrParentQuorum)
	require.ErrorIs(t, d.CheckParents(2, nil), dag.ErrParentQuorum)
	err = d.CheckParents(2, []types.CertificateDigest{round1[0], round1[1], round1[2], round1[0]})
	require.Error(t, err, "a header naming a parent twice must not be voted for")
	require.NotErrorIs(t, err, dag.ErrParentQuorum)
	require.Contains(t, err.Error(), "twice")
}
