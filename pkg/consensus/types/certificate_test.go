// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types_test

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func makeCommittee(t *testing.T, n int) (*types.Committee, []ed25519.PrivateKey) {
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

func createSignedHeader(t *testing.T, pub ed25519.PublicKey, priv ed25519.PrivateKey, round types.Round, epoch uint64) *types.Header {
	t.Helper()
	header := types.NewHeader(pub, round, epoch, nil, nil)
	err := header.Sign(priv)
	require.NoError(t, err)
	return header
}

func createCertificate(t *testing.T, header *types.Header, committee *types.Committee, privKeys []ed25519.PrivateKey, signerIndices []int) *types.Certificate {
	t.Helper()

	cert := &types.Certificate{
		Header: *header,
	}

	headerDigest := header.Digest()

	for _, idx := range signerIndices {
		sig := ed25519.Sign(privKeys[idx], headerDigest[:])
		cert.AddSignature(uint16(idx), sig)
	}

	return cert
}

func TestCertificate_Digest(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	header := createSignedHeader(t, pub, priv, 1, 0)

	cert := &types.Certificate{
		Header: *header,
	}

	// Certificate digest should match header digest
	assert.Equal(t, types.CertificateDigest(header.Digest()), cert.Digest())
}

func TestCertificate_Accessors(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	parents := []types.CertificateDigest{{1, 2, 3}}
	header := types.NewHeader(pub, 5, 2, nil, parents)
	err := header.Sign(priv)
	require.NoError(t, err)

	cert := &types.Certificate{Header: *header}

	assert.Equal(t, types.Round(5), cert.Round())
	assert.Equal(t, uint64(2), cert.Epoch())
	assert.Equal(t, pub, cert.Author())
	assert.Len(t, cert.Parents(), 1)
}

func TestCertificate_Verify(t *testing.T) {
	committee, privKeys := makeCommittee(t, 4)
	authorIdx := 0
	pub := committee.Validators[authorIdx].PublicKey
	priv := privKeys[authorIdx]

	t.Run("valid certificate", func(t *testing.T) {
		header := createSignedHeader(t, pub, priv, 1, 0)
		cert := createCertificate(t, header, committee, privKeys, []int{0, 1, 2}) // 3 of 4 = quorum

		err := cert.Verify(committee)
		assert.NoError(t, err)
	})

	t.Run("all validators signed", func(t *testing.T) {
		header := createSignedHeader(t, pub, priv, 1, 0)
		cert := createCertificate(t, header, committee, privKeys, []int{0, 1, 2, 3})

		err := cert.Verify(committee)
		assert.NoError(t, err)
	})

	t.Run("not enough signatures", func(t *testing.T) {
		header := createSignedHeader(t, pub, priv, 1, 0)
		cert := createCertificate(t, header, committee, privKeys, []int{0, 1}) // Only 2 of 4

		err := cert.Verify(committee)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "quorum")
	})

	t.Run("nil committee", func(t *testing.T) {
		header := createSignedHeader(t, pub, priv, 1, 0)
		cert := createCertificate(t, header, committee, privKeys, []int{0, 1, 2})

		err := cert.Verify(nil)
		assert.Error(t, err)
	})

	t.Run("epoch mismatch", func(t *testing.T) {
		header := createSignedHeader(t, pub, priv, 1, 5) // Different epoch
		cert := createCertificate(t, header, committee, privKeys, []int{0, 1, 2})

		err := cert.Verify(committee)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "epoch")
	})

	t.Run("author not in committee", func(t *testing.T) {
		outsiderPub, outsiderPriv, _ := ed25519.GenerateKey(nil)
		header := createSignedHeader(t, outsiderPub, outsiderPriv, 1, 0)
		cert := createCertificate(t, header, committee, privKeys, []int{0, 1, 2})

		err := cert.Verify(committee)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "author")
	})

	t.Run("invalid signature", func(t *testing.T) {
		header := createSignedHeader(t, pub, priv, 1, 0)
		cert := createCertificate(t, header, committee, privKeys, []int{0, 1, 2})

		// Corrupt a signature
		cert.Signatures[0][0] ^= 0xFF

		err := cert.Verify(committee)
		assert.Error(t, err)
	})

	t.Run("invalid authority index", func(t *testing.T) {
		header := createSignedHeader(t, pub, priv, 1, 0)
		cert := &types.Certificate{
			Header:            *header,
			Signatures:        [][]byte{make([]byte, 64)},
			SignedAuthorities: []uint16{100}, // Out of bounds
		}

		err := cert.Verify(committee)
		assert.Error(t, err)
	})

	t.Run("duplicate signer", func(t *testing.T) {
		header := createSignedHeader(t, pub, priv, 1, 0)
		headerDigest := header.Digest()

		cert := &types.Certificate{
			Header: *header,
			Signatures: [][]byte{
				ed25519.Sign(privKeys[0], headerDigest[:]),
				ed25519.Sign(privKeys[0], headerDigest[:]),
				ed25519.Sign(privKeys[1], headerDigest[:]),
			},
			SignedAuthorities: []uint16{0, 0, 1}, // Duplicate 0
		}

		err := cert.Verify(committee)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate")
	})

	t.Run("no signatures", func(t *testing.T) {
		header := createSignedHeader(t, pub, priv, 1, 0)
		cert := &types.Certificate{
			Header:            *header,
			Signatures:        nil,
			SignedAuthorities: nil,
		}

		err := cert.Verify(committee)
		assert.Error(t, err)
	})
}

func TestCertificate_TotalStake(t *testing.T) {
	committee, privKeys := makeCommittee(t, 4)
	pub := committee.Validators[0].PublicKey
	priv := privKeys[0]

	header := createSignedHeader(t, pub, priv, 1, 0)
	cert := createCertificate(t, header, committee, privKeys, []int{0, 2})

	stake := cert.TotalStake(committee)
	assert.Equal(t, uint64(200), stake) // 2 validators * 100 stake each
}

func TestCertificate_Marshal(t *testing.T) {
	committee, privKeys := makeCommittee(t, 4)
	pub := committee.Validators[0].PublicKey
	priv := privKeys[0]

	t.Run("round trip", func(t *testing.T) {
		header := createSignedHeader(t, pub, priv, 1, 0)
		cert := createCertificate(t, header, committee, privKeys, []int{0, 1, 2})

		data, err := cert.Marshal()
		require.NoError(t, err)

		unmarshaled, err := types.UnmarshalCertificate(data)
		require.NoError(t, err)

		// Verify unmarshaled certificate
		err = unmarshaled.Verify(committee)
		assert.NoError(t, err)

		assert.Equal(t, cert.Digest(), unmarshaled.Digest())
		assert.Equal(t, cert.Round(), unmarshaled.Round())
		assert.Equal(t, len(cert.Signatures), len(unmarshaled.Signatures))
	})

	t.Run("digest preserved", func(t *testing.T) {
		header := createSignedHeader(t, pub, priv, 1, 0)
		cert := createCertificate(t, header, committee, privKeys, []int{0, 1, 2})
		originalDigest := cert.Digest()

		data, err := cert.Marshal()
		require.NoError(t, err)

		unmarshaled, err := types.UnmarshalCertificate(data)
		require.NoError(t, err)

		assert.Equal(t, originalDigest, unmarshaled.Digest())
	})
}

func TestCertificate_UnmarshalErrors(t *testing.T) {
	t.Run("too short", func(t *testing.T) {
		_, err := types.UnmarshalCertificate([]byte{1, 2, 3})
		assert.Error(t, err)
	})

	t.Run("trailing bytes", func(t *testing.T) {
		committee, privKeys := makeCommittee(t, 4)
		pub := committee.Validators[0].PublicKey
		priv := privKeys[0]

		header := createSignedHeader(t, pub, priv, 1, 0)
		cert := createCertificate(t, header, committee, privKeys, []int{0, 1, 2})

		data, _ := cert.Marshal()
		data = append(data, 0xFF)

		_, err := types.UnmarshalCertificate(data)
		assert.Error(t, err)
	})
}

func TestCertificate_Clone(t *testing.T) {
	committee, privKeys := makeCommittee(t, 4)
	pub := committee.Validators[0].PublicKey
	priv := privKeys[0]

	header := createSignedHeader(t, pub, priv, 1, 0)
	original := createCertificate(t, header, committee, privKeys, []int{0, 1, 2})

	clone := original.Clone()

	assert.Equal(t, original.Digest(), clone.Digest())
	assert.Equal(t, len(original.Signatures), len(clone.Signatures))

	// Modify clone shouldn't affect original
	clone.Signatures[0][0] ^= 0xFF
	assert.NotEqual(t, original.Signatures[0], clone.Signatures[0])
}

func TestCertificate_AddSignature(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	header := createSignedHeader(t, pub, priv, 1, 0)

	cert := &types.Certificate{
		Header: *header,
	}

	sig1 := make([]byte, 64)
	sig2 := make([]byte, 64)
	sig3 := make([]byte, 64)

	// Add in non-sorted order
	cert.AddSignature(5, sig2)
	cert.AddSignature(2, sig1)
	cert.AddSignature(10, sig3)

	// Should be sorted
	assert.Equal(t, []uint16{2, 5, 10}, cert.SignedAuthorities)
	assert.Equal(t, 3, len(cert.Signatures))

	// Adding duplicate should be no-op
	cert.AddSignature(5, sig2)
	assert.Equal(t, 3, len(cert.Signatures))
}

func TestCertificate_HasSignatureFrom(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	header := createSignedHeader(t, pub, priv, 1, 0)

	cert := &types.Certificate{
		Header: *header,
	}

	cert.AddSignature(2, make([]byte, 64))
	cert.AddSignature(5, make([]byte, 64))
	cert.AddSignature(10, make([]byte, 64))

	assert.True(t, cert.HasSignatureFrom(2))
	assert.True(t, cert.HasSignatureFrom(5))
	assert.True(t, cert.HasSignatureFrom(10))
	assert.False(t, cert.HasSignatureFrom(0))
	assert.False(t, cert.HasSignatureFrom(3))
	assert.False(t, cert.HasSignatureFrom(100))
}
