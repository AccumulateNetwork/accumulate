// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types_test

import (
	"crypto/ed25519"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func generateKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	return pub, priv
}

func TestHeader_Digest(t *testing.T) {
	pub, _ := generateKeyPair(t)

	t.Run("deterministic", func(t *testing.T) {
		header1 := types.NewHeader(pub, 1, 0, nil, nil)
		header2 := types.NewHeader(pub, 1, 0, nil, nil)

		assert.Equal(t, header1.Digest(), header2.Digest())
	})

	t.Run("different round", func(t *testing.T) {
		header1 := types.NewHeader(pub, 1, 0, nil, nil)
		header2 := types.NewHeader(pub, 2, 0, nil, nil)

		assert.NotEqual(t, header1.Digest(), header2.Digest())
	})

	t.Run("different epoch", func(t *testing.T) {
		header1 := types.NewHeader(pub, 1, 0, nil, nil)
		header2 := types.NewHeader(pub, 1, 1, nil, nil)

		assert.NotEqual(t, header1.Digest(), header2.Digest())
	})

	t.Run("different author", func(t *testing.T) {
		pub2, _ := generateKeyPair(t)
		header1 := types.NewHeader(pub, 1, 0, nil, nil)
		header2 := types.NewHeader(pub2, 1, 0, nil, nil)

		assert.NotEqual(t, header1.Digest(), header2.Digest())
	})

	t.Run("with payload", func(t *testing.T) {
		batch := types.NewBatch([][]byte{[]byte("test")})
		payload := []types.PayloadEntry{
			{Digest: batch.Digest(), Worker: 0},
		}

		header1 := types.NewHeader(pub, 1, 0, nil, nil)
		header2 := types.NewHeader(pub, 1, 0, payload, nil)

		assert.NotEqual(t, header1.Digest(), header2.Digest())
	})

	t.Run("payload order independent", func(t *testing.T) {
		batch1 := types.NewBatch([][]byte{[]byte("test1")})
		batch2 := types.NewBatch([][]byte{[]byte("test2")})

		// Create payloads in different order
		payload1 := []types.PayloadEntry{
			{Digest: batch1.Digest(), Worker: 0},
			{Digest: batch2.Digest(), Worker: 1},
		}
		payload2 := []types.PayloadEntry{
			{Digest: batch2.Digest(), Worker: 1},
			{Digest: batch1.Digest(), Worker: 0},
		}

		header1 := types.NewHeader(pub, 1, 0, payload1, nil)
		header2 := types.NewHeader(pub, 1, 0, payload2, nil)

		// Should be same because map ordering shouldn't matter
		assert.Equal(t, header1.Digest(), header2.Digest())
	})

	t.Run("caching", func(t *testing.T) {
		header := types.NewHeader(pub, 1, 0, nil, nil)
		digest1 := header.Digest()
		digest2 := header.Digest()
		assert.Equal(t, digest1, digest2)
	})
}

func TestHeader_SignAndVerify(t *testing.T) {
	pub, priv := generateKeyPair(t)

	t.Run("sign and verify", func(t *testing.T) {
		header := types.NewHeader(pub, 1, 0, nil, nil)

		err := header.Sign(priv)
		require.NoError(t, err)

		err = header.Verify()
		assert.NoError(t, err)
	})

	t.Run("wrong key", func(t *testing.T) {
		pub2, priv2 := generateKeyPair(t)
		_ = pub2

		header := types.NewHeader(pub, 1, 0, nil, nil)

		err := header.Sign(priv2)
		assert.Error(t, err)
	})

	t.Run("wrong signature fails verification", func(t *testing.T) {
		header1 := types.NewHeader(pub, 1, 0, nil, nil)
		header2 := types.NewHeader(pub, 2, 0, nil, nil)

		// Sign header1
		err := header1.Sign(priv)
		require.NoError(t, err)

		// Copy header1's signature to header2 (wrong signature for header2)
		header2.Signature = make([]byte, len(header1.Signature))
		copy(header2.Signature, header1.Signature)

		// Verification should fail because signature doesn't match header2's digest
		err = header2.Verify()
		assert.Error(t, err)
	})

	t.Run("unsigned header fails verification", func(t *testing.T) {
		header := types.NewHeader(pub, 1, 0, nil, nil)
		err := header.Verify()
		assert.Error(t, err)
	})

	t.Run("invalid signature size", func(t *testing.T) {
		header := types.NewHeader(pub, 1, 0, nil, nil)
		header.Signature = []byte{1, 2, 3} // Too short

		err := header.Verify()
		assert.Error(t, err)
	})
}

func TestHeader_Marshal(t *testing.T) {
	pub, priv := generateKeyPair(t)

	t.Run("round trip unsigned", func(t *testing.T) {
		header := types.NewHeader(pub, 5, 2, nil, nil)

		data, err := header.Marshal()
		require.NoError(t, err)

		unmarshaled, err := types.UnmarshalHeader(data)
		require.NoError(t, err)

		assert.Equal(t, header.Round, unmarshaled.Round)
		assert.Equal(t, header.Epoch, unmarshaled.Epoch)
		assert.Equal(t, []byte(header.Author), []byte(unmarshaled.Author))
	})

	t.Run("round trip signed", func(t *testing.T) {
		header := types.NewHeader(pub, 5, 2, nil, nil)
		err := header.Sign(priv)
		require.NoError(t, err)

		data, err := header.Marshal()
		require.NoError(t, err)

		unmarshaled, err := types.UnmarshalHeader(data)
		require.NoError(t, err)

		err = unmarshaled.Verify()
		assert.NoError(t, err)
	})

	t.Run("with payload and parents", func(t *testing.T) {
		batch := types.NewBatch([][]byte{[]byte("test")})
		payload := []types.PayloadEntry{
			{Digest: batch.Digest(), Worker: 5},
		}
		parents := []types.CertificateDigest{{1, 2, 3}}

		header := types.NewHeader(pub, 1, 0, payload, parents)
		err := header.Sign(priv)
		require.NoError(t, err)

		data, err := header.Marshal()
		require.NoError(t, err)

		unmarshaled, err := types.UnmarshalHeader(data)
		require.NoError(t, err)

		assert.Len(t, unmarshaled.Payload, 1)
		assert.Len(t, unmarshaled.Parents, 1)
		assert.Equal(t, batch.Digest(), unmarshaled.Payload[0].Digest)
		assert.Equal(t, types.WorkerID(5), unmarshaled.Payload[0].Worker)

		err = unmarshaled.Verify()
		assert.NoError(t, err)
	})

	t.Run("digest preserved", func(t *testing.T) {
		header := types.NewHeader(pub, 1, 0, nil, nil)
		originalDigest := header.Digest()

		data, err := header.Marshal()
		require.NoError(t, err)

		unmarshaled, err := types.UnmarshalHeader(data)
		require.NoError(t, err)

		assert.Equal(t, originalDigest, unmarshaled.Digest())
	})
}

func TestHeader_UnmarshalErrors(t *testing.T) {
	t.Run("too short", func(t *testing.T) {
		_, err := types.UnmarshalHeader([]byte{1, 2, 3})
		assert.Error(t, err)
	})

	t.Run("truncated", func(t *testing.T) {
		pub, _ := generateKeyPair(t)
		header := types.NewHeader(pub, 1, 0, nil, nil)
		data, _ := header.Marshal()

		_, err := types.UnmarshalHeader(data[:len(data)-5])
		assert.Error(t, err)
	})

	t.Run("trailing bytes", func(t *testing.T) {
		pub, _ := generateKeyPair(t)
		header := types.NewHeader(pub, 1, 0, nil, nil)
		data, _ := header.Marshal()
		data = append(data, 0xFF)

		_, err := types.UnmarshalHeader(data)
		assert.Error(t, err)
	})
}

func TestHeader_Clone(t *testing.T) {
	pub, priv := generateKeyPair(t)
	batch := types.NewBatch([][]byte{[]byte("test")})
	payload := []types.PayloadEntry{
		{Digest: batch.Digest(), Worker: 1},
	}
	parents := []types.CertificateDigest{{1, 2, 3}}

	original := types.NewHeader(pub, 5, 2, payload, parents)
	err := original.Sign(priv)
	require.NoError(t, err)

	clone := original.Clone()

	// Same values
	assert.Equal(t, original.Digest(), clone.Digest())
	assert.Equal(t, original.Round, clone.Round)
	assert.Equal(t, original.Epoch, clone.Epoch)

	// But different underlying memory
	clone.Round = 999
	assert.NotEqual(t, original.Round, clone.Round)
}

func TestHeader_ConcurrentDigest(t *testing.T) {
	pub, _ := generateKeyPair(t)
	header := types.NewHeader(pub, 1, 0, nil, nil)

	var wg sync.WaitGroup
	digests := make([]types.HeaderDigest, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			digests[idx] = header.Digest()
		}(i)
	}

	wg.Wait()

	for i := 1; i < len(digests); i++ {
		assert.Equal(t, digests[0], digests[i])
	}
}

func TestHeaderDigest_String(t *testing.T) {
	pub, _ := generateKeyPair(t)
	header := types.NewHeader(pub, 1, 0, nil, nil)
	digest := header.Digest()

	str := digest.String()
	assert.Len(t, str, 64)
}
