// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// relayChallengeTag is the domain the relay's challenge lives in.
//
// The validator key also signs headers, votes, certificates and anchors, so
// answering a challenge is a signing oracle unless what is signed cannot be
// any of those. Every signature this key makes elsewhere is over a 32-byte
// digest; a challenge is signed over the WHOLE preimage, which begins with
// this ASCII tag and is far longer, so nothing produced here can be replayed
// as consensus output and nothing consensus produces answers a challenge.
const relayChallengeTag = "accumulate-relay-challenge-v1|"

// relayChallengeSize is the nonce length. Fresh per attempt, never reused:
// a remembered signature would let one lie answer every later challenge.
const relayChallengeSize = 32

// newRelayChallenge returns a fresh nonce.
func newRelayChallenge() ([]byte, error) {
	b := make([]byte, relayChallengeSize)
	if _, err := rand.Read(b); err != nil {
		return nil, errors.InternalError.WithFormat("read random: %w", err)
	}
	return b, nil
}

// relayChallengeMessage is what a candidate signs: the tag, the partition it
// is answering for, the key hash it claims, and the caller's nonce.
//
// The partition is in it because a node may hold a committee key for one
// partition and none for another, and a signature made for one must not
// stand for the other. The claimed hash is in it because the answer must
// bind to the identity the relay then checks against its own globals, not
// merely to the nonce.
func relayChallengeMessage(partition string, keyHash [32]byte, nonce []byte) []byte {
	m := make([]byte, 0, len(relayChallengeTag)+len(partition)+1+len(keyHash)+len(nonce))
	m = append(m, relayChallengeTag...)
	m = append(m, partition...)
	m = append(m, '|')
	m = append(m, keyHash[:]...)
	m = append(m, nonce...)
	return m
}

// signRelayChallenge answers a challenge with the validator key. An empty
// nonce is not answered: a node does not sign what it was not asked.
func signRelayChallenge(key ed25519.PrivateKey, partition string, nonce []byte) []byte {
	if len(key) != ed25519.PrivateKeySize || len(nonce) == 0 {
		return nil
	}
	pub := key.Public().(ed25519.PublicKey)
	return ed25519.Sign(key, relayChallengeMessage(partition, sha256.Sum256(pub), nonce))
}

// verifyRelayChallenge reports whether sig is the answer this partition's
// validator holding keyHash would give to nonce.
//
// key comes from THIS node's globals, found by the hash the candidate
// claimed — so a peer that names a validator's hash must also hold that
// validator's key, which is the whole point (#4366 F1, lead note_3869952619).
func verifyRelayChallenge(key ed25519.PublicKey, partition string, keyHash [32]byte, nonce, sig []byte) bool {
	if len(key) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize || len(nonce) == 0 {
		return false
	}
	return ed25519.Verify(key, relayChallengeMessage(partition, keyHash, nonce), sig)
}
