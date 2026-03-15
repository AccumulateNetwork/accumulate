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

func TestVote_SignAndVerify(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	headerDigest := types.HeaderDigest{1, 2, 3, 4, 5}

	t.Run("sign and verify", func(t *testing.T) {
		vote := types.NewVote(headerDigest, 5, 2, pub)

		err := vote.Sign(priv)
		require.NoError(t, err)

		err = vote.Verify()
		assert.NoError(t, err)
	})

	t.Run("wrong key", func(t *testing.T) {
		pub2, priv2, _ := ed25519.GenerateKey(nil)
		_ = pub2

		vote := types.NewVote(headerDigest, 5, 2, pub)

		err := vote.Sign(priv2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not match")
	})

	t.Run("unsigned vote fails verification", func(t *testing.T) {
		vote := types.NewVote(headerDigest, 5, 2, pub)
		err := vote.Verify()
		assert.Error(t, err)
	})

	t.Run("modified vote fails verification", func(t *testing.T) {
		vote := types.NewVote(headerDigest, 5, 2, pub)
		err := vote.Sign(priv)
		require.NoError(t, err)

		// Modify the vote
		vote.Round = 999

		err = vote.Verify()
		assert.Error(t, err)
	})

	t.Run("invalid signature size", func(t *testing.T) {
		vote := types.NewVote(headerDigest, 5, 2, pub)
		vote.Signature = []byte{1, 2, 3} // Too short

		err := vote.Verify()
		assert.Error(t, err)
	})

	t.Run("invalid author key size", func(t *testing.T) {
		vote := types.NewVote(headerDigest, 5, 2, []byte{1, 2, 3})
		vote.Signature = make([]byte, ed25519.SignatureSize)

		err := vote.Verify()
		assert.Error(t, err)
	})
}

func TestVote_VerifyForHeader(t *testing.T) {
	authorPub, authorPriv, _ := ed25519.GenerateKey(nil)
	voterPub, voterPriv, _ := ed25519.GenerateKey(nil)

	header := types.NewHeader(authorPub, 5, 2, nil, nil)
	err := header.Sign(authorPriv)
	require.NoError(t, err)

	t.Run("valid vote for header", func(t *testing.T) {
		vote := types.NewVote(header.Digest(), header.Round, header.Epoch, voterPub)
		err := vote.Sign(voterPriv)
		require.NoError(t, err)

		err = vote.VerifyForHeader(header)
		assert.NoError(t, err)
	})

	t.Run("wrong header digest", func(t *testing.T) {
		wrongDigest := types.HeaderDigest{1, 2, 3}
		vote := types.NewVote(wrongDigest, header.Round, header.Epoch, voterPub)
		err := vote.Sign(voterPriv)
		require.NoError(t, err)

		err = vote.VerifyForHeader(header)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "digest")
	})

	t.Run("wrong round", func(t *testing.T) {
		vote := types.NewVote(header.Digest(), 999, header.Epoch, voterPub)
		err := vote.Sign(voterPriv)
		require.NoError(t, err)

		err = vote.VerifyForHeader(header)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "round")
	})

	t.Run("wrong epoch", func(t *testing.T) {
		vote := types.NewVote(header.Digest(), header.Round, 999, voterPub)
		err := vote.Sign(voterPriv)
		require.NoError(t, err)

		err = vote.VerifyForHeader(header)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "epoch")
	})
}

func TestVote_Marshal(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	headerDigest := types.HeaderDigest{1, 2, 3, 4, 5, 6, 7, 8}

	t.Run("round trip unsigned", func(t *testing.T) {
		vote := types.NewVote(headerDigest, 5, 2, pub)

		data, err := vote.Marshal()
		require.NoError(t, err)

		unmarshaled, err := types.UnmarshalVote(data)
		require.NoError(t, err)

		assert.Equal(t, vote.HeaderDigest, unmarshaled.HeaderDigest)
		assert.Equal(t, vote.Round, unmarshaled.Round)
		assert.Equal(t, vote.Epoch, unmarshaled.Epoch)
		assert.Equal(t, []byte(vote.Author), []byte(unmarshaled.Author))
	})

	t.Run("round trip signed", func(t *testing.T) {
		vote := types.NewVote(headerDigest, 5, 2, pub)
		err := vote.Sign(priv)
		require.NoError(t, err)

		data, err := vote.Marshal()
		require.NoError(t, err)

		unmarshaled, err := types.UnmarshalVote(data)
		require.NoError(t, err)

		err = unmarshaled.Verify()
		assert.NoError(t, err)
	})
}

func TestVote_UnmarshalErrors(t *testing.T) {
	t.Run("too short", func(t *testing.T) {
		_, err := types.UnmarshalVote([]byte{1, 2, 3})
		assert.Error(t, err)
	})

	t.Run("truncated signature", func(t *testing.T) {
		pub, priv, _ := ed25519.GenerateKey(nil)
		vote := types.NewVote(types.HeaderDigest{}, 1, 0, pub)
		err := vote.Sign(priv)
		require.NoError(t, err)

		data, _ := vote.Marshal()
		// Truncate the signature
		_, err = types.UnmarshalVote(data[:len(data)-10])
		assert.Error(t, err)
	})

	t.Run("trailing bytes", func(t *testing.T) {
		pub, priv, _ := ed25519.GenerateKey(nil)
		vote := types.NewVote(types.HeaderDigest{}, 1, 0, pub)
		err := vote.Sign(priv)
		require.NoError(t, err)

		data, _ := vote.Marshal()
		data = append(data, 0xFF)

		_, err = types.UnmarshalVote(data)
		assert.Error(t, err)
	})
}

func TestVote_Clone(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	headerDigest := types.HeaderDigest{1, 2, 3}

	original := types.NewVote(headerDigest, 5, 2, pub)
	err := original.Sign(priv)
	require.NoError(t, err)

	clone := original.Clone()

	// Same values
	assert.Equal(t, original.HeaderDigest, clone.HeaderDigest)
	assert.Equal(t, original.Round, clone.Round)
	assert.Equal(t, original.Epoch, clone.Epoch)

	// But different underlying memory
	clone.Round = 999
	assert.NotEqual(t, original.Round, clone.Round)

	clone.Signature[0] ^= 0xFF
	assert.NotEqual(t, original.Signature, clone.Signature)
}

func TestVote_VoteDigest(t *testing.T) {
	pub1, _, _ := ed25519.GenerateKey(nil)
	pub2, _, _ := ed25519.GenerateKey(nil)
	headerDigest := types.HeaderDigest{1, 2, 3}

	t.Run("same vote produces same digest", func(t *testing.T) {
		vote1 := types.NewVote(headerDigest, 5, 2, pub1)
		vote2 := types.NewVote(headerDigest, 5, 2, pub1)

		assert.Equal(t, vote1.VoteDigest(), vote2.VoteDigest())
	})

	t.Run("different author produces different digest", func(t *testing.T) {
		vote1 := types.NewVote(headerDigest, 5, 2, pub1)
		vote2 := types.NewVote(headerDigest, 5, 2, pub2)

		assert.NotEqual(t, vote1.VoteDigest(), vote2.VoteDigest())
	})

	t.Run("different header produces different digest", func(t *testing.T) {
		vote1 := types.NewVote(types.HeaderDigest{1}, 5, 2, pub1)
		vote2 := types.NewVote(types.HeaderDigest{2}, 5, 2, pub1)

		assert.NotEqual(t, vote1.VoteDigest(), vote2.VoteDigest())
	})
}
