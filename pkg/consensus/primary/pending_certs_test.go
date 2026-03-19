// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func TestPendingCertificatesAdd(t *testing.T) {
	pc := NewPendingCertificates(10)

	// Create test validators and certificates
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)

	// Create a certificate
	header := types.NewHeader(validators[0].pub, 1, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))

	sigs := make([][]byte, len(validators))
	authors := make([]uint16, len(validators))
	for i, v := range validators {
		vote := types.NewVote(header.Digest(), 1, 1, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		sigs[i] = vote.Signature
		authors[i] = uint16(i)
	}
	cert := types.NewCertificate(header, sigs, authors)

	// Create fake parent digests
	var parent1, parent2 types.CertificateDigest
	copy(parent1[:], []byte("parent1_digest_here_____"))
	copy(parent2[:], []byte("parent2_digest_here_____"))

	// Add certificate with missing parents
	added := pc.Add(cert, []types.CertificateDigest{parent1, parent2})
	require.True(t, added)
	require.Equal(t, 1, pc.Size())
	require.True(t, pc.Contains(cert.Digest()))

	// Try to add again - should fail (already exists)
	added = pc.Add(cert, []types.CertificateDigest{parent1})
	require.False(t, added)
	require.Equal(t, 1, pc.Size())

	// Check metrics
	buffered, inserted, pruned := pc.Metrics()
	require.Equal(t, uint64(1), buffered)
	require.Equal(t, uint64(0), inserted)
	require.Equal(t, uint64(0), pruned)

	// Verify we can retrieve the certificate
	retrieved := pc.Get(cert.Digest())
	require.NotNil(t, retrieved)
	require.Equal(t, cert.Digest(), retrieved.Digest())

	// Verify missing parents are tracked
	missing := pc.MissingParents(cert.Digest())
	require.Len(t, missing, 2)
	_ = committee // silence unused
}

func TestPendingCertificatesAddNil(t *testing.T) {
	pc := NewPendingCertificates(10)

	// nil certificate
	added := pc.Add(nil, []types.CertificateDigest{{1, 2, 3}})
	require.False(t, added)
	require.Equal(t, 0, pc.Size())

	// Create a certificate but with no missing parents
	v := newTestValidator(t)
	header := types.NewHeader(v.pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(v.priv))
	cert := types.NewCertificate(header, nil, nil)

	added = pc.Add(cert, nil)
	require.False(t, added)
	require.Equal(t, 0, pc.Size())

	added = pc.Add(cert, []types.CertificateDigest{})
	require.False(t, added)
	require.Equal(t, 0, pc.Size())
}

func TestPendingCertificatesMaxSize(t *testing.T) {
	maxSize := 3
	pc := NewPendingCertificates(maxSize)

	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	var parent types.CertificateDigest
	copy(parent[:], []byte("shared_parent_digest____"))

	// Add certificates up to max size
	for i := 0; i < maxSize; i++ {
		header := types.NewHeader(validators[i%len(validators)].pub, types.Round(i+1), 1, nil, nil)
		require.NoError(t, header.Sign(validators[i%len(validators)].priv))

		sigs := make([][]byte, len(validators))
		authors := make([]uint16, len(validators))
		for j, v := range validators {
			vote := types.NewVote(header.Digest(), types.Round(i+1), 1, v.pub)
			require.NoError(t, vote.Sign(v.priv))
			sigs[j] = vote.Signature
			authors[j] = uint16(j)
		}
		cert := types.NewCertificate(header, sigs, authors)

		added := pc.Add(cert, []types.CertificateDigest{parent})
		require.True(t, added, "should be able to add certificate %d", i)
	}

	require.Equal(t, maxSize, pc.Size())

	// Try to add one more - should fail
	header := types.NewHeader(validators[0].pub, 100, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))
	cert := types.NewCertificate(header, nil, nil)

	added := pc.Add(cert, []types.CertificateDigest{parent})
	require.False(t, added)
	require.Equal(t, maxSize, pc.Size())
}

func TestPendingCertificatesOnParentAvailable(t *testing.T) {
	pc := NewPendingCertificates(10)

	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	// Create parent digests
	var parent1, parent2 types.CertificateDigest
	copy(parent1[:], []byte("parent1_digest_here_____"))
	copy(parent2[:], []byte("parent2_digest_here_____"))

	// Create certificate waiting for both parents
	header := types.NewHeader(validators[0].pub, 1, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))

	sigs := make([][]byte, len(validators))
	authors := make([]uint16, len(validators))
	for i, v := range validators {
		vote := types.NewVote(header.Digest(), 1, 1, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		sigs[i] = vote.Signature
		authors[i] = uint16(i)
	}
	cert := types.NewCertificate(header, sigs, authors)

	added := pc.Add(cert, []types.CertificateDigest{parent1, parent2})
	require.True(t, added)
	require.Equal(t, 1, pc.Size())

	// Parent 1 becomes available - certificate should NOT be ready (still needs parent2)
	ready := pc.OnParentAvailable(parent1)
	require.Empty(t, ready)
	require.Equal(t, 1, pc.Size()) // Still pending

	// Parent 2 becomes available - certificate should be ready now
	ready = pc.OnParentAvailable(parent2)
	require.Len(t, ready, 1)
	require.Equal(t, cert.Digest(), ready[0].Digest())
	require.Equal(t, 0, pc.Size()) // No longer pending

	// Check metrics
	buffered, inserted, pruned := pc.Metrics()
	require.Equal(t, uint64(1), buffered)
	require.Equal(t, uint64(1), inserted)
	require.Equal(t, uint64(0), pruned)
}

func TestPendingCertificatesMultipleWaiters(t *testing.T) {
	pc := NewPendingCertificates(10)

	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	// Create a shared parent
	var sharedParent types.CertificateDigest
	copy(sharedParent[:], []byte("shared_parent_digest____"))

	// Create multiple certificates waiting for the same parent
	certs := make([]*types.Certificate, 3)
	for i := 0; i < 3; i++ {
		header := types.NewHeader(validators[i].pub, types.Round(i+1), 1, nil, nil)
		require.NoError(t, header.Sign(validators[i].priv))

		sigs := make([][]byte, len(validators))
		authors := make([]uint16, len(validators))
		for j, v := range validators {
			vote := types.NewVote(header.Digest(), types.Round(i+1), 1, v.pub)
			require.NoError(t, vote.Sign(v.priv))
			sigs[j] = vote.Signature
			authors[j] = uint16(j)
		}
		certs[i] = types.NewCertificate(header, sigs, authors)

		added := pc.Add(certs[i], []types.CertificateDigest{sharedParent})
		require.True(t, added)
	}

	require.Equal(t, 3, pc.Size())

	// Shared parent becomes available - all should be ready
	ready := pc.OnParentAvailable(sharedParent)
	require.Len(t, ready, 3)
	require.Equal(t, 0, pc.Size())

	// Check metrics
	buffered, inserted, _ := pc.Metrics()
	require.Equal(t, uint64(3), buffered)
	require.Equal(t, uint64(3), inserted)
}

func TestPendingCertificatesPrune(t *testing.T) {
	pc := NewPendingCertificates(100)

	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	var parent types.CertificateDigest
	copy(parent[:], []byte("parent_digest_here______"))

	// Add certificates at various rounds
	rounds := []types.Round{1, 5, 10, 15, 20}
	for i, round := range rounds {
		header := types.NewHeader(validators[i%len(validators)].pub, round, 1, nil, nil)
		require.NoError(t, header.Sign(validators[i%len(validators)].priv))

		sigs := make([][]byte, len(validators))
		authors := make([]uint16, len(validators))
		for j, v := range validators {
			vote := types.NewVote(header.Digest(), round, 1, v.pub)
			require.NoError(t, vote.Sign(v.priv))
			sigs[j] = vote.Signature
			authors[j] = uint16(j)
		}
		cert := types.NewCertificate(header, sigs, authors)

		added := pc.Add(cert, []types.CertificateDigest{parent})
		require.True(t, added)
	}

	require.Equal(t, 5, pc.Size())

	// Prune with currentRound=25, gcDepth=10: cutoff=15
	// Should prune rounds < 15: 1, 5, 10
	pruned := pc.Prune(25, 10)
	require.Equal(t, 3, pruned)
	require.Equal(t, 2, pc.Size()) // rounds 15 and 20 remain

	// Check metrics
	_, _, totalPruned := pc.Metrics()
	require.Equal(t, uint64(3), totalPruned)
}

func TestPendingCertificatesPruneEdgeCases(t *testing.T) {
	pc := NewPendingCertificates(10)

	// Prune when empty
	pruned := pc.Prune(100, 10)
	require.Equal(t, 0, pruned)

	// Prune when currentRound <= gcDepth
	pruned = pc.Prune(5, 10)
	require.Equal(t, 0, pruned)

	pruned = pc.Prune(10, 10)
	require.Equal(t, 0, pruned)
}

func TestPendingCertificatesOnParentAvailableNonExistent(t *testing.T) {
	pc := NewPendingCertificates(10)

	// Call OnParentAvailable with a parent that nobody is waiting for
	var unknownParent types.CertificateDigest
	copy(unknownParent[:], []byte("unknown_parent__________"))

	ready := pc.OnParentAvailable(unknownParent)
	require.Empty(t, ready)
}

func TestPendingCertificatesGetNonExistent(t *testing.T) {
	pc := NewPendingCertificates(10)

	var unknownDigest types.CertificateDigest
	copy(unknownDigest[:], []byte("unknown_digest__________"))

	cert := pc.Get(unknownDigest)
	require.Nil(t, cert)

	missing := pc.MissingParents(unknownDigest)
	require.Nil(t, missing)
}

func TestPendingCertificatesContains(t *testing.T) {
	pc := NewPendingCertificates(10)

	v := newTestValidator(t)
	header := types.NewHeader(v.pub, 1, 1, nil, nil)
	require.NoError(t, header.Sign(v.priv))
	cert := types.NewCertificate(header, nil, nil)

	var parent types.CertificateDigest
	copy(parent[:], []byte("parent__________________"))

	require.False(t, pc.Contains(cert.Digest()))

	pc.Add(cert, []types.CertificateDigest{parent})
	require.True(t, pc.Contains(cert.Digest()))
}

func TestIntegrationBufferAndRecoverMissingParent(t *testing.T) {
	// Integration test: simulate a node receiving certificates out of order
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition:           "test",
		KeyPair:             validators[0].priv,
		NewCertsChannelSize: 10,
	}

	p := New(config, committee, nil, d, nil)

	// Create genesis certificates for all validators
	genesisCerts := createGenesisCertificates(t, validators, committee, d)

	// Create round 1 certificates
	round1Certs := make([]*types.Certificate, len(validators))
	for i, v := range validators {
		parents := make([]types.CertificateDigest, len(genesisCerts))
		for j, gc := range genesisCerts {
			parents[j] = gc.Digest()
		}

		header := types.NewHeader(v.pub, 1, 1, nil, parents)
		require.NoError(t, header.Sign(v.priv))

		sigs := make([][]byte, len(validators))
		authors := make([]uint16, len(validators))
		for j, voter := range validators {
			vote := types.NewVote(header.Digest(), 1, 1, voter.pub)
			require.NoError(t, vote.Sign(voter.priv))
			sigs[j] = vote.Signature
			authors[j] = uint16(j)
		}
		round1Certs[i] = types.NewCertificate(header, sigs, authors)
	}

	// Create a round 2 certificate that references round 1 certificates
	round1Digests := make([]types.CertificateDigest, len(round1Certs))
	for i, c := range round1Certs {
		round1Digests[i] = c.Digest()
	}

	header := types.NewHeader(validators[0].pub, 2, 1, nil, round1Digests)
	require.NoError(t, header.Sign(validators[0].priv))

	sigs := make([][]byte, len(validators))
	authors := make([]uint16, len(validators))
	for i, v := range validators {
		vote := types.NewVote(header.Digest(), 2, 1, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		sigs[i] = vote.Signature
		authors[i] = uint16(i)
	}
	round2Cert := types.NewCertificate(header, sigs, authors)

	// Receive round 2 certificate FIRST (out of order)
	// This should buffer it because round 1 parents are missing
	p.OnCertificateReceived(round2Cert)

	// Verify it's buffered (not in DAG)
	require.Nil(t, d.GetByDigest(round2Cert.Digest()))
	require.Equal(t, 1, p.PendingCertsCount())

	// Now receive round 1 certificates
	for _, cert := range round1Certs {
		p.OnCertificateReceived(cert)
	}

	// Verify round 1 certificates are in DAG
	for _, cert := range round1Certs {
		require.NotNil(t, d.GetByDigest(cert.Digest()))
	}

	// After receiving the last parent, round 2 cert should be automatically inserted
	require.NotNil(t, d.GetByDigest(round2Cert.Digest()))
	require.Equal(t, 0, p.PendingCertsCount())

	// Check metrics
	buffered, inserted, pruned := p.PendingCertsMetrics()
	require.Equal(t, uint64(1), buffered)
	require.Equal(t, uint64(1), inserted)
	require.Equal(t, uint64(0), pruned)
}

func TestIntegrationChainedPendingCertificates(t *testing.T) {
	// Test scenario: Round 3 arrives, then Round 2, then Round 1
	// When Round 1 arrives, both Round 2 and Round 3 should be inserted
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition:           "test",
		KeyPair:             validators[0].priv,
		NewCertsChannelSize: 10,
	}

	p := New(config, committee, nil, d, nil)

	// Create genesis certificates
	genesisCerts := createGenesisCertificates(t, validators, committee, d)
	genesisDigests := make([]types.CertificateDigest, len(genesisCerts))
	for i, c := range genesisCerts {
		genesisDigests[i] = c.Digest()
	}

	// Create round 1 certificate
	header1 := types.NewHeader(validators[0].pub, 1, 1, nil, genesisDigests)
	require.NoError(t, header1.Sign(validators[0].priv))

	sigs1 := make([][]byte, len(validators))
	authors1 := make([]uint16, len(validators))
	for i, v := range validators {
		vote := types.NewVote(header1.Digest(), 1, 1, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		sigs1[i] = vote.Signature
		authors1[i] = uint16(i)
	}
	round1Cert := types.NewCertificate(header1, sigs1, authors1)

	// Create round 2 certificate referencing round 1
	header2 := types.NewHeader(validators[0].pub, 2, 1, nil, []types.CertificateDigest{round1Cert.Digest()})
	require.NoError(t, header2.Sign(validators[0].priv))

	sigs2 := make([][]byte, len(validators))
	authors2 := make([]uint16, len(validators))
	for i, v := range validators {
		vote := types.NewVote(header2.Digest(), 2, 1, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		sigs2[i] = vote.Signature
		authors2[i] = uint16(i)
	}
	round2Cert := types.NewCertificate(header2, sigs2, authors2)

	// Create round 3 certificate referencing round 2
	header3 := types.NewHeader(validators[0].pub, 3, 1, nil, []types.CertificateDigest{round2Cert.Digest()})
	require.NoError(t, header3.Sign(validators[0].priv))

	sigs3 := make([][]byte, len(validators))
	authors3 := make([]uint16, len(validators))
	for i, v := range validators {
		vote := types.NewVote(header3.Digest(), 3, 1, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		sigs3[i] = vote.Signature
		authors3[i] = uint16(i)
	}
	round3Cert := types.NewCertificate(header3, sigs3, authors3)

	// Receive certificates in reverse order: Round 3, then Round 2, then Round 1
	p.OnCertificateReceived(round3Cert)
	require.Nil(t, d.GetByDigest(round3Cert.Digest()))
	require.Equal(t, 1, p.PendingCertsCount())

	p.OnCertificateReceived(round2Cert)
	require.Nil(t, d.GetByDigest(round2Cert.Digest()))
	require.Equal(t, 2, p.PendingCertsCount())

	// Receive round 1 - this should trigger a chain: insert round1 -> insert round2 -> insert round3
	p.OnCertificateReceived(round1Cert)

	// All certificates should now be in the DAG
	require.NotNil(t, d.GetByDigest(round1Cert.Digest()))
	require.NotNil(t, d.GetByDigest(round2Cert.Digest()))
	require.NotNil(t, d.GetByDigest(round3Cert.Digest()))
	require.Equal(t, 0, p.PendingCertsCount())
}
