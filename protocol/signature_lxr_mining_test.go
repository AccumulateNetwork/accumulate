// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"crypto/ed25519"
	"crypto/rand"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func TestLXRMiningSignature(t *testing.T) {
	t.Run("Basic Mining", func(t *testing.T) {
		// Generate a key pair
		pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		// Create a test transaction
		txn := &Transaction{
			Header: TransactionHeader{
				Principal: url.MustParse("acc://alice.acme"),
			},
			Body: &SendTokens{
				To: []*TokenRecipient{{
					Url:    url.MustParse("acc://bob.acme"),
					Amount: *big.NewInt(100),
				}},
			},
		}

		// Create mining signature
		sig := &LXRMiningSignature{
			PublicKey:     pubKey,
			Signer:        url.MustParse("acc://miner.acme/book/1"),
			SignerVersion: 1,
			Timestamp:     uint64(time.Now().Unix()),
			TableSize:     20, // Small table for testing (1MB instead of 1GB)
			TableSeed:     0xFAFAECECFAFAECEC,
			Passes:        5,
		}

		// Mine with low difficulty for testing
		err = sig.Mine(txn, 1000) // Low difficulty for quick testing
		require.NoError(t, err)
		t.Logf("Found nonce: %d", sig.Nonce)
		t.Logf("Work proof: %x", sig.WorkProof)

		// Verify mining proof
		require.True(t, sig.VerifyMining(txn))

		// Sign the work proof
		err = SignLXRMining(sig, privKey)
		require.NoError(t, err)

		// Verify complete signature
		require.True(t, sig.Verify(sig, txn))
	})

	t.Run("Invalid Mining Proof", func(t *testing.T) {
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		txn := &Transaction{
			Header: TransactionHeader{
				Principal: url.MustParse("acc://alice.acme"),
			},
			Body: &SendTokens{
				To: []*TokenRecipient{{
					Url:    url.MustParse("acc://bob.acme"),
					Amount: *big.NewInt(100),
				}},
			},
		}

		sig := &LXRMiningSignature{
			PublicKey:     pubKey,
			Signer:        url.MustParse("acc://miner.acme/book/1"),
			SignerVersion: 1,
			Timestamp:     uint64(time.Now().Unix()),
			Nonce:         12345,      // Random nonce
			Difficulty:    1000,
			WorkProof:     [32]byte{}, // Invalid proof
		}

		// Should fail verification
		require.False(t, sig.VerifyMining(txn))
	})

	t.Run("Difficulty Scaling", func(t *testing.T) {
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		txn := &Transaction{
			Header: TransactionHeader{
				Principal: url.MustParse("acc://alice.acme"),
			},
			Body: &SendTokens{
				To: []*TokenRecipient{{
					Url:    url.MustParse("acc://bob.acme"),
					Amount: *big.NewInt(100),
				}},
			},
		}

		// Test with different difficulties
		difficulties := []uint64{100, 500, 1000}
		var miningTimes []time.Duration

		for _, diff := range difficulties {
			sig := &LXRMiningSignature{
				PublicKey:     pubKey,
				Signer:        url.MustParse("acc://miner.acme/book/1"),
				SignerVersion: 1,
				Timestamp:     uint64(time.Now().Unix()),
			}

			start := time.Now()
			err := sig.Mine(txn, diff)
			elapsed := time.Since(start)

			require.NoError(t, err)
			require.True(t, sig.VerifyMining(txn))

			miningTimes = append(miningTimes, elapsed)
			t.Logf("Difficulty %d: nonce=%d, time=%v", diff, sig.Nonce, elapsed)
		}

		// Higher difficulty should generally take more time
		// (though randomness means this isn't guaranteed for small samples)
		t.Logf("Mining times: %v", miningTimes)
	})

	t.Run("Metadata and Initiator", func(t *testing.T) {
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		sig := &LXRMiningSignature{
			PublicKey:       pubKey,
			Signer:          url.MustParse("acc://miner.acme/book/1"),
			SignerVersion:   1,
			Timestamp:       uint64(time.Now().Unix()),
			Nonce:           12345,
			Difficulty:      1000,
			WorkProof:       [32]byte{1, 2, 3},
			Signature:       []byte{4, 5, 6},
			TransactionHash: [32]byte{7, 8, 9},
		}

		// Test Metadata - should exclude Signature and TransactionHash
		metadata := sig.Metadata().(*LXRMiningSignature)
		require.Nil(t, metadata.Signature)
		require.Equal(t, [32]byte{}, metadata.TransactionHash)
		require.Equal(t, sig.Nonce, metadata.Nonce)
		require.Equal(t, sig.WorkProof, metadata.WorkProof)

		// Test Initiator
		initiator, err := sig.Initiator()
		require.NoError(t, err)
		require.NotNil(t, initiator)

		// Should produce a consistent hash
		hash1 := initiator.MerkleHash()
		hash2 := initiator.MerkleHash()
		require.Equal(t, hash1, hash2)
	})

	t.Run("Anti-Spam Use Case", func(t *testing.T) {
		// Simulate anti-spam: require mining proof for account creation
		pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		// Create account transaction
		txn := &Transaction{
			Header: TransactionHeader{
				Principal: url.MustParse("acc://alice.acme"),
			},
			Body: &CreateIdentity{
				Url: url.MustParse("acc://alice.acme"),
			},
		}

		// Mine with anti-spam difficulty
		antiSpamDifficulty := uint64(5000)
		sig := &LXRMiningSignature{
			PublicKey:     pubKey,
			Signer:        url.MustParse("acc://miner.acme/book/1"),
			SignerVersion: 1,
			Timestamp:     uint64(time.Now().Unix()),
			Memo:          "Creating new identity",
		}

		start := time.Now()
		err = sig.Mine(txn, antiSpamDifficulty)
		require.NoError(t, err)
		t.Logf("Anti-spam mining took %v", time.Since(start))

		// Sign and verify
		err = SignLXRMining(sig, privKey)
		require.NoError(t, err)
		require.True(t, sig.Verify(sig, txn))

		// Verify difficulty meets anti-spam requirement
		require.GreaterOrEqual(t, sig.Difficulty, antiSpamDifficulty)
	})

	t.Run("GettersAndType", func(t *testing.T) {
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		sig := &LXRMiningSignature{
			PublicKey:       pubKey,
			Signature:       []byte("test-signature"),
			Signer:          url.MustParse("acc://miner.acme/book/1"),
			SignerVersion:   42,
			Timestamp:       12345,
			Vote:            VoteTypeAccept,
			TransactionHash: [32]byte{1, 2, 3},
			Memo:            "test memo",
			Data:            []byte("test data"),
		}

		// Test Type
		require.Equal(t, SignatureTypeLXRMining, sig.Type())

		// Test RoutingLocation
		require.Equal(t, "acc://miner.acme/book/1", sig.RoutingLocation().String())

		// Test getters
		require.Equal(t, VoteTypeAccept, sig.GetVote())
		require.Equal(t, "acc://miner.acme/book/1", sig.GetSigner().String())
		require.Equal(t, [32]byte{1, 2, 3}, sig.GetTransactionHash())
		require.Equal(t, []byte("test-signature"), sig.GetSignature())
		require.Equal(t, []byte(pubKey), sig.GetPublicKey())
		require.Equal(t, uint64(42), sig.GetSignerVersion())
		require.Equal(t, uint64(12345), sig.GetTimestamp())

		// Test GetPublicKeyHash
		hash := sig.GetPublicKeyHash()
		require.NotNil(t, hash)
		require.Len(t, hash, 32)

		// Test Hash
		sigHash := sig.Hash()
		require.NotNil(t, sigHash)
		require.Len(t, sigHash, 32)
	})

	t.Run("InvalidSignature", func(t *testing.T) {
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		// Generate a different key for wrong signature
		_, wrongPrivKey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		txn := &Transaction{
			Header: TransactionHeader{
				Principal: url.MustParse("acc://alice.acme"),
			},
			Body: &SendTokens{
				To: []*TokenRecipient{{
					Url:    url.MustParse("acc://bob.acme"),
					Amount: *big.NewInt(100),
				}},
			},
		}

		sig := &LXRMiningSignature{
			PublicKey:     pubKey,
			Signer:        url.MustParse("acc://miner.acme/book/1"),
			SignerVersion: 1,
			Timestamp:     uint64(time.Now().Unix()),
		}

		// Mine correctly
		err = sig.Mine(txn, 1000)
		require.NoError(t, err)

		// Sign with wrong key
		err = SignLXRMining(sig, wrongPrivKey)
		require.NoError(t, err)

		// Should fail verification
		require.False(t, sig.Verify(sig, txn))
	})

	t.Run("EmptyPublicKey", func(t *testing.T) {
		sig := &LXRMiningSignature{
			Signer:        url.MustParse("acc://miner.acme/book/1"),
			SignerVersion: 1,
		}

		txn := &Transaction{
			Header: TransactionHeader{
				Principal: url.MustParse("acc://alice.acme"),
			},
			Body: &SendTokens{
				To: []*TokenRecipient{{
					Url:    url.MustParse("acc://bob.acme"),
					Amount: *big.NewInt(100),
				}},
			},
		}

		// Should fail to mine without public key
		err := sig.Mine(txn, 1000)
		require.Error(t, err)
		require.Contains(t, err.Error(), "public key is required")

		// Should return nil for GetPublicKeyHash
		require.Nil(t, sig.GetPublicKeyHash())
	})

	t.Run("CannotInitiate", func(t *testing.T) {
		// Test with missing PublicKey
		sig := &LXRMiningSignature{
			Signer: url.MustParse("acc://miner.acme/book/1"),
		}
		_, err := sig.Initiator()
		require.ErrorIs(t, err, ErrCannotInitiate)

		// Test with missing Signer
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		sig = &LXRMiningSignature{
			PublicKey: pubKey,
		}
		_, err = sig.Initiator()
		require.ErrorIs(t, err, ErrCannotInitiate)
	})
}

func BenchmarkLXRMining(b *testing.B) {
	pubKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(b, err)

	txn := &Transaction{
		Header: TransactionHeader{
			Principal: url.MustParse("acc://alice.acme"),
		},
		Body: &SendTokens{
			To: []*TokenRecipient{{
				Url:    url.MustParse("acc://bob.acme"),
				Amount: *big.NewInt(100),
			}},
		},
	}

	b.Run("Difficulty=100", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sig := &LXRMiningSignature{
				PublicKey:     pubKey,
				Signer:        url.MustParse("acc://miner.acme/book/1"),
				SignerVersion: 1,
				Timestamp:     uint64(time.Now().Unix()),
			}
			sig.Mine(txn, 100)
		}
	})

	b.Run("Difficulty=1000", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sig := &LXRMiningSignature{
				PublicKey:     pubKey,
				Signer:        url.MustParse("acc://miner.acme/book/1"),
				SignerVersion: 1,
				Timestamp:     uint64(time.Now().Unix()),
			}
			sig.Mine(txn, 1000)
		}
	})
}