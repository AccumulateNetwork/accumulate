// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dag_test

import (
	"crypto/ed25519"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func makeTestCommittee(t *testing.T, n int) (*types.Committee, []ed25519.PrivateKey) {
	t.Helper()
	validators := make([]types.ValidatorInfo, n)
	privKeys := make([]ed25519.PrivateKey, n)

	for i := 0; i < n; i++ {
		pub, priv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		validators[i] = types.ValidatorInfo{
			PublicKey: pub,
			Stake:     100,
		}
		privKeys[i] = priv
	}

	return types.NewCommittee(validators, 0), privKeys
}

func createTestCert(t *testing.T, committee *types.Committee, privKeys []ed25519.PrivateKey, authorIdx int, round types.Round, parents []types.CertificateDigest) *types.Certificate {
	t.Helper()

	pub := committee.Validators[authorIdx].PublicKey
	priv := privKeys[authorIdx]

	header := types.NewHeader(pub, round, 0, nil, parents)
	err := header.Sign(priv)
	require.NoError(t, err)

	cert := types.NewCertificate(*header, nil, nil)

	// Add signatures from enough validators for quorum
	headerDigest := header.Digest()
	quorumCount := (len(privKeys)*2)/3 + 1
	for i := 0; i < quorumCount; i++ {
		sig := ed25519.Sign(privKeys[i], headerDigest[:])
		cert.AddSignature(uint16(i), sig)
	}

	return cert
}

func TestDAG_NewDAG(t *testing.T) {
	d := dag.NewDAG(10)
	require.NotNil(t, d)

	assert.Equal(t, types.Round(0), d.LatestRound())
	assert.Equal(t, 0, d.Size())
}

func TestDAG_InsertGenesis(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d := dag.NewDAG(10)

	// Insert genesis certificates (round 0, no parents)
	for i := 0; i < 4; i++ {
		cert := createTestCert(t, committee, privKeys, i, 0, nil)
		err := d.InsertGenesis(cert)
		assert.NoError(t, err)
	}

	assert.Equal(t, 4, d.Size())
	assert.Equal(t, types.Round(0), d.LatestRound())
}

func TestDAG_Insert(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d := dag.NewDAG(10)

	// Insert genesis certificates
	genesisCerts := make([]*types.Certificate, 4)
	for i := 0; i < 4; i++ {
		cert := createTestCert(t, committee, privKeys, i, 0, nil)
		err := d.InsertGenesis(cert)
		require.NoError(t, err)
		genesisCerts[i] = cert
	}

	// Insert round 1 certificates with parents
	parentDigests := make([]types.CertificateDigest, 4)
	for i, cert := range genesisCerts {
		parentDigests[i] = cert.Digest()
	}

	for i := 0; i < 4; i++ {
		cert := createTestCert(t, committee, privKeys, i, 1, parentDigests)
		err := d.Insert(cert)
		assert.NoError(t, err)
	}

	assert.Equal(t, 8, d.Size())
	assert.Equal(t, types.Round(1), d.LatestRound())
}

func TestDAG_InsertDuplicate(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d := dag.NewDAG(10)

	cert := createTestCert(t, committee, privKeys, 0, 0, nil)

	err := d.InsertGenesis(cert)
	require.NoError(t, err)

	// Try to insert duplicate
	err = d.InsertGenesis(cert)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestDAG_InsertMissingParent(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d := dag.NewDAG(10)

	// Try to insert certificate with non-existent parent
	fakeParent := types.CertificateDigest{1, 2, 3, 4, 5}
	cert := createTestCert(t, committee, privKeys, 0, 1, []types.CertificateDigest{fakeParent})

	err := d.Insert(cert)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parent")
}

func TestDAG_Get(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d := dag.NewDAG(10)

	cert := createTestCert(t, committee, privKeys, 2, 0, nil)
	err := d.InsertGenesis(cert)
	require.NoError(t, err)

	t.Run("found", func(t *testing.T) {
		found := d.Get(0, committee.Validators[2].PublicKey)
		require.NotNil(t, found)
		assert.Equal(t, cert.Digest(), found.Digest())
	})

	t.Run("not found - wrong round", func(t *testing.T) {
		found := d.Get(1, committee.Validators[2].PublicKey)
		assert.Nil(t, found)
	})

	t.Run("not found - wrong author", func(t *testing.T) {
		found := d.Get(0, committee.Validators[0].PublicKey)
		assert.Nil(t, found)
	})
}

func TestDAG_GetByDigest(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d := dag.NewDAG(10)

	cert := createTestCert(t, committee, privKeys, 0, 0, nil)
	err := d.InsertGenesis(cert)
	require.NoError(t, err)

	t.Run("found", func(t *testing.T) {
		found := d.GetByDigest(cert.Digest())
		require.NotNil(t, found)
		assert.Equal(t, cert.Digest(), found.Digest())
	})

	t.Run("not found", func(t *testing.T) {
		fakeDigest := types.CertificateDigest{1, 2, 3}
		found := d.GetByDigest(fakeDigest)
		assert.Nil(t, found)
	})
}

func TestDAG_GetRound(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d := dag.NewDAG(10)

	// Insert certificates for multiple rounds
	for i := 0; i < 4; i++ {
		cert := createTestCert(t, committee, privKeys, i, 0, nil)
		err := d.InsertGenesis(cert)
		require.NoError(t, err)
	}

	t.Run("existing round", func(t *testing.T) {
		certs := d.GetRound(0)
		assert.Len(t, certs, 4)
	})

	t.Run("non-existing round", func(t *testing.T) {
		certs := d.GetRound(99)
		assert.Nil(t, certs)
	})
}

func TestDAG_HasQuorum(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d := dag.NewDAG(10)

	t.Run("no quorum - empty round", func(t *testing.T) {
		assert.False(t, d.HasQuorum(0, committee))
	})

	t.Run("no quorum - not enough certs", func(t *testing.T) {
		// Insert only 2 certificates
		for i := 0; i < 2; i++ {
			cert := createTestCert(t, committee, privKeys, i, 0, nil)
			err := d.InsertGenesis(cert)
			require.NoError(t, err)
		}
		assert.False(t, d.HasQuorum(0, committee))
	})

	t.Run("has quorum", func(t *testing.T) {
		// Insert 2 more certificates (total 4, but need > 2/3)
		for i := 2; i < 4; i++ {
			cert := createTestCert(t, committee, privKeys, i, 0, nil)
			err := d.InsertGenesis(cert)
			require.NoError(t, err)
		}
		// 4 validators * 100 stake = 400 total
		// Quorum threshold = 267 (> 2/3)
		// 4 certs = 400 stake, which is >= 267
		assert.True(t, d.HasQuorum(0, committee))
	})
}

func TestDAG_GarbageCollect(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	gcDepth := types.Round(3)
	d := dag.NewDAG(gcDepth)

	// Insert certificates for rounds 0-10
	var prevParents []types.CertificateDigest
	for round := types.Round(0); round <= 10; round++ {
		var currentCerts []*types.Certificate
		for i := 0; i < 4; i++ {
			var cert *types.Certificate
			if round == 0 {
				cert = createTestCert(t, committee, privKeys, i, round, nil)
				err := d.InsertGenesis(cert)
				require.NoError(t, err)
			} else {
				cert = createTestCert(t, committee, privKeys, i, round, prevParents)
				err := d.Insert(cert)
				require.NoError(t, err)
			}
			currentCerts = append(currentCerts, cert)
		}
		prevParents = make([]types.CertificateDigest, len(currentCerts))
		for i, c := range currentCerts {
			prevParents[i] = c.Digest()
		}
	}

	assert.Equal(t, 44, d.Size()) // 11 rounds * 4 certs

	// GC at round 8 should keep rounds 5-10 (8 - 3 = 5)
	d.GarbageCollect(8)

	// Should have rounds 5-10 = 6 rounds * 4 certs = 24 certs
	assert.Equal(t, 24, d.Size())

	// Verify old rounds are gone
	assert.Nil(t, d.GetRound(0))
	assert.Nil(t, d.GetRound(4))

	// Verify new rounds still exist
	assert.NotNil(t, d.GetRound(5))
	assert.NotNil(t, d.GetRound(10))
}

func TestDAG_Contains(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d := dag.NewDAG(10)

	cert := createTestCert(t, committee, privKeys, 0, 0, nil)
	err := d.InsertGenesis(cert)
	require.NoError(t, err)

	assert.True(t, d.Contains(cert.Digest()))
	assert.False(t, d.Contains(types.CertificateDigest{1, 2, 3}))
}

func TestDAG_ContainsRoundAuthor(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d := dag.NewDAG(10)

	cert := createTestCert(t, committee, privKeys, 0, 0, nil)
	err := d.InsertGenesis(cert)
	require.NoError(t, err)

	assert.True(t, d.ContainsRoundAuthor(0, committee.Validators[0].PublicKey))
	assert.False(t, d.ContainsRoundAuthor(0, committee.Validators[1].PublicKey))
	assert.False(t, d.ContainsRoundAuthor(1, committee.Validators[0].PublicKey))
}

func TestDAG_GetParents(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d := dag.NewDAG(10)

	// Create genesis certificates
	genesisCerts := make([]*types.Certificate, 4)
	for i := 0; i < 4; i++ {
		cert := createTestCert(t, committee, privKeys, i, 0, nil)
		err := d.InsertGenesis(cert)
		require.NoError(t, err)
		genesisCerts[i] = cert
	}

	// Create round 1 certificate with parents
	parentDigests := make([]types.CertificateDigest, 4)
	for i, c := range genesisCerts {
		parentDigests[i] = c.Digest()
	}
	round1Cert := createTestCert(t, committee, privKeys, 0, 1, parentDigests)
	err := d.Insert(round1Cert)
	require.NoError(t, err)

	t.Run("has parents", func(t *testing.T) {
		parents := d.GetParents(round1Cert)
		assert.Len(t, parents, 4)
	})

	t.Run("genesis has no parents", func(t *testing.T) {
		parents := d.GetParents(genesisCerts[0])
		assert.Nil(t, parents)
	})

	t.Run("nil certificate", func(t *testing.T) {
		parents := d.GetParents(nil)
		assert.Nil(t, parents)
	})
}

func TestDAG_ConcurrentAccess(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d := dag.NewDAG(10)

	// Insert genesis certificates
	for i := 0; i < 4; i++ {
		cert := createTestCert(t, committee, privKeys, i, 0, nil)
		err := d.InsertGenesis(cert)
		require.NoError(t, err)
	}

	var wg sync.WaitGroup

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = d.GetRound(0)
			_ = d.LatestRound()
			_ = d.Size()
			_ = d.HasQuorum(0, committee)
		}()
	}

	wg.Wait()
}

func TestDAG_Rounds(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d := dag.NewDAG(10)

	// Insert certificates for rounds 0, 1, 2
	cert0 := createTestCert(t, committee, privKeys, 0, 0, nil)
	err := d.InsertGenesis(cert0)
	require.NoError(t, err)

	cert1 := createTestCert(t, committee, privKeys, 1, 1, []types.CertificateDigest{cert0.Digest()})
	err = d.Insert(cert1)
	require.NoError(t, err)

	cert2 := createTestCert(t, committee, privKeys, 2, 2, []types.CertificateDigest{cert1.Digest()})
	err = d.Insert(cert2)
	require.NoError(t, err)

	rounds := d.Rounds()
	assert.Len(t, rounds, 3)
	// Order is not guaranteed, but all should be present
	roundSet := make(map[types.Round]bool)
	for _, r := range rounds {
		roundSet[r] = true
	}
	assert.True(t, roundSet[0])
	assert.True(t, roundSet[1])
	assert.True(t, roundSet[2])
}

func TestDAG_RoundCount(t *testing.T) {
	committee, privKeys := makeTestCommittee(t, 4)
	d := dag.NewDAG(10)

	// Insert 2 certificates for round 0
	for i := 0; i < 2; i++ {
		cert := createTestCert(t, committee, privKeys, i, 0, nil)
		err := d.InsertGenesis(cert)
		require.NoError(t, err)
	}

	assert.Equal(t, 2, d.RoundCount(0))
	assert.Equal(t, 0, d.RoundCount(1))
}
