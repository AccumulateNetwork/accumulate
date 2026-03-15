// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dag_test

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// buildTestDAG creates a simple test DAG with the following structure:
//
//	Round 0: [A0] [B0] [C0] [D0]  (4 genesis certs)
//	Round 1: [A1] [B1] [C1] [D1]  (each references all round 0)
//	Round 2: [A2] [B2] [C2] [D2]  (each references all round 1)
func buildTestDAG(t *testing.T, committee *types.Committee, privKeys []ed25519.PrivateKey, numRounds int) (*dag.DAG, [][]*types.Certificate) {
	t.Helper()

	d := dag.NewDAG(20)
	allCerts := make([][]*types.Certificate, numRounds)

	for round := 0; round < numRounds; round++ {
		roundCerts := make([]*types.Certificate, len(privKeys))

		// Get parents from previous round
		var parents []types.CertificateDigest
		if round > 0 {
			for _, c := range allCerts[round-1] {
				parents = append(parents, c.Digest())
			}
		}

		for i := 0; i < len(privKeys); i++ {
			cert := createTestCert(t, committee, privKeys, i, types.Round(round), parents)
			if round == 0 {
				err := d.InsertGenesis(cert)
				require.NoError(t, err)
			} else {
				err := d.Insert(cert)
				require.NoError(t, err)
			}
			roundCerts[i] = cert
		}

		allCerts[round] = roundCerts
	}

	return d, allCerts
}

func TestDAG_IsAncestor(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d, allCerts := buildTestDAG(t, committee, privKeys, 5)

	t.Run("direct parent is ancestor", func(t *testing.T) {
		parent := allCerts[1][0]
		child := allCerts[2][0]
		assert.True(t, d.IsAncestor(parent, child))
	})

	t.Run("grandparent is ancestor", func(t *testing.T) {
		grandparent := allCerts[0][0]
		grandchild := allCerts[2][0]
		assert.True(t, d.IsAncestor(grandparent, grandchild))
	})

	t.Run("deep ancestor", func(t *testing.T) {
		genesis := allCerts[0][0]
		latest := allCerts[4][0]
		assert.True(t, d.IsAncestor(genesis, latest))
	})

	t.Run("same certificate", func(t *testing.T) {
		cert := allCerts[2][0]
		assert.True(t, d.IsAncestor(cert, cert))
	})

	t.Run("not ancestor - wrong direction", func(t *testing.T) {
		parent := allCerts[2][0]
		child := allCerts[1][0]
		assert.False(t, d.IsAncestor(parent, child))
	})

	t.Run("nil certificates", func(t *testing.T) {
		assert.False(t, d.IsAncestor(nil, allCerts[0][0]))
		assert.False(t, d.IsAncestor(allCerts[0][0], nil))
		assert.False(t, d.IsAncestor(nil, nil))
	})
}

func TestDAG_GetAncestors(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d, allCerts := buildTestDAG(t, committee, privKeys, 4)

	t.Run("get all ancestors", func(t *testing.T) {
		cert := allCerts[3][0]
		ancestors := d.GetAncestors(cert, 0)

		// Should include cert itself and all reachable ancestors
		// Round 3: 1 cert, Round 2: 4 certs, Round 1: 4 certs, Round 0: 4 certs = 13
		assert.Len(t, ancestors, 13)
	})

	t.Run("limited by minRound", func(t *testing.T) {
		cert := allCerts[3][0]
		ancestors := d.GetAncestors(cert, 2)

		// Should include round 3 (1) and round 2 (4) = 5
		assert.Len(t, ancestors, 5)
	})

	t.Run("genesis has only itself", func(t *testing.T) {
		genesis := allCerts[0][0]
		ancestors := d.GetAncestors(genesis, 0)
		assert.Len(t, ancestors, 1)
		assert.Equal(t, genesis.Digest(), ancestors[0].Digest())
	})

	t.Run("nil certificate", func(t *testing.T) {
		ancestors := d.GetAncestors(nil, 0)
		assert.Nil(t, ancestors)
	})
}

func TestDAG_TopologicalSort(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d, allCerts := buildTestDAG(t, committee, privKeys, 4)

	t.Run("sorts by round", func(t *testing.T) {
		// Mix certificates from different rounds
		unsorted := []*types.Certificate{
			allCerts[2][0],
			allCerts[0][0],
			allCerts[3][0],
			allCerts[1][0],
		}

		sorted := d.TopologicalSort(unsorted)
		require.Len(t, sorted, 4)

		// Parents should come before children
		assert.Equal(t, types.Round(0), sorted[0].Round())
		assert.Equal(t, types.Round(1), sorted[1].Round())
		assert.Equal(t, types.Round(2), sorted[2].Round())
		assert.Equal(t, types.Round(3), sorted[3].Round())
	})

	t.Run("empty input", func(t *testing.T) {
		sorted := d.TopologicalSort(nil)
		assert.Nil(t, sorted)

		sorted = d.TopologicalSort([]*types.Certificate{})
		assert.Nil(t, sorted)
	})

	t.Run("single certificate", func(t *testing.T) {
		sorted := d.TopologicalSort([]*types.Certificate{allCerts[0][0]})
		assert.Len(t, sorted, 1)
	})

	t.Run("multiple in same round", func(t *testing.T) {
		// All from round 0
		unsorted := []*types.Certificate{
			allCerts[0][2],
			allCerts[0][0],
			allCerts[0][3],
			allCerts[0][1],
		}

		sorted := d.TopologicalSort(unsorted)
		require.Len(t, sorted, 4)

		// All should be round 0, sorted deterministically by digest
		for _, cert := range sorted {
			assert.Equal(t, types.Round(0), cert.Round())
		}
	})

	t.Run("preserves all certificates", func(t *testing.T) {
		// Get all ancestors of a round 3 certificate
		ancestors := d.GetAncestors(allCerts[3][0], 0)

		sorted := d.TopologicalSort(ancestors)

		// Should have same number of certificates
		assert.Len(t, sorted, len(ancestors))

		// All certificates should be present
		sortedSet := make(map[types.CertificateDigest]bool)
		for _, c := range sorted {
			sortedSet[c.Digest()] = true
		}
		for _, c := range ancestors {
			assert.True(t, sortedSet[c.Digest()], "Missing certificate in sorted output")
		}
	})
}

func TestDAG_GetChildren(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d, allCerts := buildTestDAG(t, committee, privKeys, 3)

	t.Run("genesis has children", func(t *testing.T) {
		genesis := allCerts[0][0]
		children := d.GetChildren(genesis)

		// All round 1 certificates should reference all genesis certificates
		assert.Len(t, children, 4)
		for _, child := range children {
			assert.Equal(t, types.Round(1), child.Round())
		}
	})

	t.Run("latest round has no children", func(t *testing.T) {
		latest := allCerts[2][0]
		children := d.GetChildren(latest)
		assert.Len(t, children, 0)
	})

	t.Run("nil certificate", func(t *testing.T) {
		children := d.GetChildren(nil)
		assert.Nil(t, children)
	})
}

func TestDAG_GetDescendants(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d, allCerts := buildTestDAG(t, committee, privKeys, 4)

	t.Run("genesis descendants", func(t *testing.T) {
		genesis := allCerts[0][0]
		descendants := d.GetDescendants(genesis, 3)

		// Should include genesis itself and all reachable descendants
		// Genesis is referenced by all round 1, which are referenced by all round 2, etc.
		// So genesis + 4 + 4 + 4 = 13
		assert.Len(t, descendants, 13)
	})

	t.Run("limited by maxRound", func(t *testing.T) {
		genesis := allCerts[0][0]
		descendants := d.GetDescendants(genesis, 1)

		// Genesis + round 1 = 1 + 4 = 5
		assert.Len(t, descendants, 5)
	})

	t.Run("latest has only itself", func(t *testing.T) {
		latest := allCerts[3][0]
		descendants := d.GetDescendants(latest, 10)
		assert.Len(t, descendants, 1)
	})

	t.Run("nil certificate", func(t *testing.T) {
		descendants := d.GetDescendants(nil, 10)
		assert.Nil(t, descendants)
	})
}

func TestDAG_FindLeader(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d, _ := buildTestDAG(t, committee, privKeys, 4)

	t.Run("finds leader for each round", func(t *testing.T) {
		for round := types.Round(0); round < 4; round++ {
			leader := d.FindLeader(round, committee)
			require.NotNil(t, leader, "Should find leader for round %d", round)

			// Leader should be deterministic based on round
			expectedIdx := int(round) % committee.Len()
			assert.Equal(t, committee.Validators[expectedIdx].PublicKey, leader.Author())
		}
	})

	t.Run("no leader in empty round", func(t *testing.T) {
		leader := d.FindLeader(99, committee)
		assert.Nil(t, leader)
	})

	t.Run("nil committee", func(t *testing.T) {
		leader := d.FindLeader(0, nil)
		assert.Nil(t, leader)
	})

	// Test that leader is from the expected validator
	t.Run("leader matches expected validator", func(t *testing.T) {
		// Round 0: validator 0
		leader0 := d.FindLeader(0, committee)
		assert.Equal(t, committee.Validators[0].PublicKey, leader0.Author())

		// Round 1: validator 1
		leader1 := d.FindLeader(1, committee)
		assert.Equal(t, committee.Validators[1].PublicKey, leader1.Author())
	})
}

func TestDAG_CountSupport(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d, allCerts := buildTestDAG(t, committee, privKeys, 4)

	t.Run("full support", func(t *testing.T) {
		// A certificate from round 2 should have support from all round 3 certificates
		// because all round 3 certs reference all round 2 certs
		cert := allCerts[2][0]
		support := d.CountSupport(cert, 3, committee)

		// All 4 validators have stake 100, so total support = 400
		assert.Equal(t, uint64(400), support)
	})

	t.Run("genesis support", func(t *testing.T) {
		genesis := allCerts[0][0]
		support := d.CountSupport(genesis, 1, committee)

		// All round 1 certificates reference all genesis certificates
		assert.Equal(t, uint64(400), support)
	})

	t.Run("no support from earlier round", func(t *testing.T) {
		cert := allCerts[2][0]
		support := d.CountSupport(cert, 1, committee) // Round before cert's round

		assert.Equal(t, uint64(0), support)
	})

	t.Run("nil certificate", func(t *testing.T) {
		support := d.CountSupport(nil, 3, committee)
		assert.Equal(t, uint64(0), support)
	})
}

// TestDAG_PartialParents tests a DAG where not all certificates reference all parents
func TestDAG_PartialParents(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d := dag.NewDAG(20)

	// Create genesis certificates
	genesisCerts := make([]*types.Certificate, 4)
	for i := 0; i < 4; i++ {
		cert := createTestCert(t, committee, privKeys, i, 0, nil)
		err := d.InsertGenesis(cert)
		require.NoError(t, err)
		genesisCerts[i] = cert
	}

	// Create round 1 certificates with partial parents
	// Cert A1 only references A0, B0 (2 parents)
	// Cert B1 references B0, C0, D0 (3 parents)
	parentsA1 := []types.CertificateDigest{
		genesisCerts[0].Digest(),
		genesisCerts[1].Digest(),
	}
	certA1 := createTestCert(t, committee, privKeys, 0, 1, parentsA1)
	err := d.Insert(certA1)
	require.NoError(t, err)

	parentsB1 := []types.CertificateDigest{
		genesisCerts[1].Digest(),
		genesisCerts[2].Digest(),
		genesisCerts[3].Digest(),
	}
	certB1 := createTestCert(t, committee, privKeys, 1, 1, parentsB1)
	err = d.Insert(certB1)
	require.NoError(t, err)

	// A1 should have A0 and B0 as ancestors
	assert.True(t, d.IsAncestor(genesisCerts[0], certA1))
	assert.True(t, d.IsAncestor(genesisCerts[1], certA1))
	assert.False(t, d.IsAncestor(genesisCerts[2], certA1))
	assert.False(t, d.IsAncestor(genesisCerts[3], certA1))

	// B1 should have B0, C0, D0 as ancestors
	assert.False(t, d.IsAncestor(genesisCerts[0], certB1))
	assert.True(t, d.IsAncestor(genesisCerts[1], certB1))
	assert.True(t, d.IsAncestor(genesisCerts[2], certB1))
	assert.True(t, d.IsAncestor(genesisCerts[3], certB1))

	// GetAncestors should return correct counts
	ancestorsA1 := d.GetAncestors(certA1, 0)
	assert.Len(t, ancestorsA1, 3) // A1 + A0 + B0

	ancestorsB1 := d.GetAncestors(certB1, 0)
	assert.Len(t, ancestorsB1, 4) // B1 + B0 + C0 + D0
}
