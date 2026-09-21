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

	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// relayChallengeTag is the domain the relay's challenge lives in.
//
// The validator key also signs headers, votes, certificates and anchors, so
// answering a challenge is a signing oracle unless what is signed cannot be
// any of those. What separates the domains is THIS PREFIX: every challenge
// preimage begins with these thirty ASCII bytes, and no consensus message
// does. Length does not separate them -- a vote signs 48 bytes and a state
// hash message 56 -- so the tag is the whole of the argument, and it is
// checked by construction because nothing else builds a preimage.
const relayChallengeTag = "accumulate-relay-challenge-v1|"

// relayChallengeSize is the nonce length the relay sends. Fresh per attempt,
// never reused: a remembered signature would let one lie answer every later
// challenge.
const relayChallengeSize = 32

// relayChallengeMaxNonce is the longest nonce a node will answer. A signer
// that signs any length is a signing oracle with an attacker-chosen
// payload length; the tag still separates the domains, but there is no
// reason to offer it.
const relayChallengeMaxNonce = 64

// newRelayChallenge returns a fresh nonce.
func newRelayChallenge() ([]byte, error) {
	b := make([]byte, relayChallengeSize)
	if _, err := rand.Read(b); err != nil {
		return nil, errors.InternalError.WithFormat("read random: %w", err)
	}
	return b, nil
}

// relayChallengeMessage is what a candidate signs: the tag, the partition it
// is answering for, the key hash it claims, THE PEER ID IT IS ANSWERING AS,
// and the caller's nonce.
//
// The partition is in it because a node may hold a committee key for one
// partition and none for another, and a signature made for one must not
// stand for the other. The claimed hash is in it because the answer must
// bind to the identity the relay then checks against its own globals.
//
// The peer ID is in it because the key alone binds the signature to a
// VALIDATOR and not to WHOEVER ANSWERED. ConsensusStatus signs any caller's
// nonce, over the p2p consensus service and the public HTTP status alike,
// so a peer with no key could take the relay's nonce, forward it to a real
// validator, and hand back that validator's answer as its own -- one extra
// round trip, well inside the deadline, and it is handed the submission
// (#4366, threat re-check note_3869991754). With the ID in the preimage the
// forwarded answer is a signature for somebody else's peer ID, and the
// relay verifies with the ID it dialled.
func relayChallengeMessage(partition string, keyHash [32]byte, id peer.ID, nonce []byte) []byte {
	// The peer ID in its printed form, because that is the form the caller
	// names it in (ConsensusStatusOptions.NodeID is a string) and the only
	// form both ends agree on without a round trip through base58.
	self := id.String()
	m := make([]byte, 0, len(relayChallengeTag)+len(partition)+len(keyHash)+len(self)+len(nonce)+2)
	m = append(m, relayChallengeTag...)
	m = append(m, partition...)
	m = append(m, '|')
	m = append(m, keyHash[:]...)
	m = append(m, self...)
	m = append(m, '|')
	m = append(m, nonce...)
	return m
}

// signRelayChallenge answers a challenge with the validator key, AS THIS
// NODE and for nobody else.
//
// self is this node's own peer ID and askedFor is the ID the caller
// addressed; they must match, or this node is being asked to mint an
// identity proof for another peer, which is the forwarding attack. An empty
// nonce is not answered either -- a node does not sign what it was not
// asked -- nor one longer than the relay would ever send.
func signRelayChallenge(key ed25519.PrivateKey, partition string, self peer.ID, askedFor string, nonce []byte) []byte {
	switch {
	case len(key) != ed25519.PrivateKeySize,
		len(nonce) == 0, len(nonce) > relayChallengeMaxNonce,
		self == "",
		askedFor != self.String():
		return nil
	}
	pub := key.Public().(ed25519.PublicKey)
	return ed25519.Sign(key, relayChallengeMessage(partition, sha256.Sum256(pub), self, nonce))
}

// verifyRelayChallenge reports whether sig is the answer the peer at id,
// holding keyHash, would give to nonce for this partition.
//
// key comes from THIS node's globals, found by the hash the candidate
// claimed, and id is the peer the relay DIALLED -- so the node that
// answered must both hold that validator's key and be the one the relay is
// about to hand the submission to (#4366 F1; the forwarding half,
// note_3869991754).
func verifyRelayChallenge(key ed25519.PublicKey, partition string, keyHash [32]byte, id peer.ID, nonce, sig []byte) bool {
	if len(key) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize ||
		len(nonce) == 0 || id == "" {
		return false
	}
	return ed25519.Verify(key, relayChallengeMessage(partition, keyHash, id, nonce), sig)
}
