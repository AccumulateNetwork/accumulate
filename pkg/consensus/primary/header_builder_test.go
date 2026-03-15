// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func TestCreateHeaderGenesisRound(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	// Genesis round should create header without parents
	header, err := p.CreateHeader()
	require.NoError(t, err)
	require.NotNil(t, header)

	require.Equal(t, v.pub, ed25519.PublicKey(header.Author))
	require.Equal(t, types.Round(0), header.Round)
	require.Equal(t, uint64(1), header.Epoch)
	require.Empty(t, header.Parents)

	// Verify signature
	require.NoError(t, header.Verify())
}

func TestCreateHeaderWithParents(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	// Create genesis certificates
	createGenesisCertificates(t, validators, committee, d)

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	// Advance to round 1
	p.SetRound(1)

	// Now we should be able to create header with parents
	header, err := p.CreateHeader()
	require.NoError(t, err)
	require.NotNil(t, header)

	require.Equal(t, types.Round(1), header.Round)
	require.Len(t, header.Parents, 4) // All 4 genesis certs as parents

	// Verify signature
	require.NoError(t, header.Verify())
}

func TestCreateHeaderNotEnoughParents(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	// Only create 1 genesis certificate (not enough for quorum)
	header := types.NewHeader(validators[0].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))

	sigs := [][]byte{make([]byte, 64)}
	authors := []uint16{0}

	cert := types.NewCertificate(*header, sigs, authors)
	require.NoError(t, d.InsertGenesis(cert))

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)
	p.SetRound(1)

	// Should fail due to not enough parents
	_, err := p.CreateHeader()
	require.ErrorIs(t, err, ErrNotEnoughParents)
}

func TestGetParentCertsGenesisRound(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	// Genesis round has no parents
	parents, err := p.GetParentCerts()
	require.NoError(t, err)
	require.Nil(t, parents)
}

func TestGetParentCertsWithCerts(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	genesisCerts := createGenesisCertificates(t, validators, committee, d)

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)
	p.SetRound(1)

	parents, err := p.GetParentCerts()
	require.NoError(t, err)
	require.Len(t, parents, 4)

	// Verify all genesis certs are parents
	genesisDigests := make(map[types.CertificateDigest]bool)
	for _, c := range genesisCerts {
		genesisDigests[c.Digest()] = true
	}

	for _, pd := range parents {
		require.True(t, genesisDigests[pd], "parent should be a genesis cert")
	}
}

func TestAvailableBatchesEmpty(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	// No workers, no batches
	batches := p.AvailableBatches()
	require.Empty(t, batches)
}

func TestCreateHeaderPayloadEmpty(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	// No workers
	p := New(config, committee, nil, d, nil)

	header, err := p.CreateHeader()
	require.NoError(t, err)
	require.NotNil(t, header)

	// Empty payload is fine
	require.Empty(t, header.Payload)
}

func TestCreateHeaderSignatureValid(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	header, err := p.CreateHeader()
	require.NoError(t, err)

	// Verify signature is valid
	require.NoError(t, header.Verify())

	// Verify signed by correct key
	require.Equal(t, v.pub, ed25519.PublicKey(header.Author))
}

func TestCreateHeaderDifferentEpochs(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}

	for _, epoch := range []uint64{1, 10, 100, 1000} {
		t.Run("epoch_"+string(rune('0'+epoch%10)), func(t *testing.T) {
			committee := newTestCommittee(validators, epoch)
			d := newTestDAG()

			config := Config{
				Partition: "test",
				KeyPair:   v.priv,
			}

			p := New(config, committee, nil, d, nil)

			header, err := p.CreateHeader()
			require.NoError(t, err)
			require.Equal(t, epoch, header.Epoch)
		})
	}
}

func TestMultipleHeaderCreationsInSameRound(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create multiple headers in same round
	header1, err := p.CreateHeader()
	require.NoError(t, err)

	header2, err := p.CreateHeader()
	require.NoError(t, err)

	// Both headers should be for round 0
	require.Equal(t, header1.Round, header2.Round)

	// Digests may differ if payload/timing differs, but round is the same
	require.Equal(t, types.Round(0), header1.Round)
	require.Equal(t, types.Round(0), header2.Round)
}

func TestCreateHeaderAcrossRounds(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	// Round 0 - no parents needed
	header0, err := p.CreateHeader()
	require.NoError(t, err)
	require.Equal(t, types.Round(0), header0.Round)
	require.Empty(t, header0.Parents)

	// Create genesis certificates for all validators
	createGenesisCertificates(t, validators, committee, d)

	// Advance to round 1
	p.SetRound(1)

	// Round 1 - should have parents
	header1, err := p.CreateHeader()
	require.NoError(t, err)
	require.Equal(t, types.Round(1), header1.Round)
	require.NotEmpty(t, header1.Parents)
}

// TestCreateHeaderConcurrency tests that CreateHeader is thread-safe.
func TestCreateHeaderConcurrency(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	validators := []*testValidator{{pub: pub, priv: priv}}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create headers concurrently
	const numGoroutines = 10
	errors := make(chan error, numGoroutines)
	headers := make(chan *types.Header, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			h, err := p.CreateHeader()
			if err != nil {
				errors <- err
				return
			}
			headers <- h
		}()
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		select {
		case err := <-errors:
			t.Fatalf("unexpected error: %v", err)
		case h := <-headers:
			require.NotNil(t, h)
			require.Equal(t, types.Round(0), h.Round)
			require.NoError(t, h.Verify())
		}
	}
}
