// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package trustbundle defines the peer-served delivery bundle that
// bootstrapping launchers fetch to acquire current state in one round
// trip (issues #3954 and #3961, parent #3953).
//
// The bundle is signed by the partition validator quorum at the bundle's
// height. The signatures are a peer-attestation primitive — they let a
// receiver trust the bundle from a single peer without round-tripping
// to a quorum of peers itself. They are NOT load-bearing for the
// proof-of-derivation guarantee; that comes from the receiver-side
// back-walk (#3960). The bundle is a delivery convenience.
//
// For backwards compatibility, peers that don't advertise a node state
// (legacy nodes, per #3970) cannot serve this bundle — bootstrapping
// launchers depend on at least one upgraded peer being reachable.
package trustbundle

import (
	"crypto/ed25519"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ErrInsufficientSignatures is returned when Verify counts fewer
// distinct verified validator signatures than the threshold rule
// requires. Sentinel for telemetry / programmatic handling (issue
// #3984).
var ErrInsufficientSignatures = errors.New("trustbundle: insufficient signatures")

// Bundle is the peer-served delivery payload.
type Bundle struct {
	// Version is the bundle's serialization version.
	Version uint32

	// Network identifies the network (e.g., "mainnet").
	Network string

	// Partition is the partition the bundle is for.
	Partition string

	// MajorBlockIndex / MinorBlockIndex identify the block H the
	// bundle was captured at (confirmed depth, not live tip).
	MajorBlockIndex uint64
	MinorBlockIndex uint64

	// MajorBlockTime is the block time of MajorBlockIndex.
	MajorBlockTimeUnix int64

	// PerPartitionAnchors carries the (RootChainAnchor, StateTreeAnchor)
	// for each partition observed at the bundle's height. Order is
	// canonical (partition URL alphabetical).
	PerPartitionAnchors []PartitionAnchorEntry

	// MinimumBootstrapSet is the current state of the accounts a launcher
	// needs to start: DN's Network account, operator key books, anchor /
	// synthetic / system ledgers, plus any configured extras.
	MinimumBootstrapSet []AccountEntry

	// ValidatorSet is the partition validator set at the bundle's height.
	ValidatorSet []ValidatorEntry

	// Signatures is a list of validator-quorum signatures over a canonical
	// hash of the bundle (excluding Signatures itself). Each signature
	// must come from a key in ValidatorSet.
	Signatures []ValidatorSignature
}

// PartitionAnchorEntry is one partition's anchor commitments at the
// bundle's height.
type PartitionAnchorEntry struct {
	Partition       string
	RootChainAnchor [32]byte
	StateTreeAnchor [32]byte
}

// AccountEntry is one account's serialized state at the bundle's height.
type AccountEntry struct {
	// Url is the account URL.
	Url *url.URL

	// State is the account's serialized form (matching protocol marshaling).
	State []byte

	// ValueHash is the hash of State that should match the BPT leaf's
	// value at the bundle's height.
	ValueHash [32]byte
}

// ValidatorEntry is one validator's identity at the bundle's height.
type ValidatorEntry struct {
	// PublicKeyHash identifies the validator.
	PublicKeyHash [32]byte

	// PublicKey is the raw public key (sufficient for verification of
	// signatures over the bundle).
	PublicKey []byte

	// Type is the signature scheme.
	Type protocol.SignatureType
}

// ValidatorSignature is one signature over the bundle from a validator.
type ValidatorSignature struct {
	PublicKeyHash [32]byte
	Signature     []byte
}

// Verify checks the bundle's structural invariants and validates each
// signature against the embedded ValidatorSet using the configured
// threshold rule. The caller must already trust the validator set
// (i.e., have back-walked it to the pinned genesis snapshot hash).
//
// Threshold rule (initial): require at least (2/3)*N signatures from
// distinct validators. Can be overridden via opts.
func (b *Bundle) Verify(opts VerifyOptions) error {
	if b == nil {
		return errors.New("nil bundle")
	}
	if len(b.ValidatorSet) == 0 {
		return errors.New("empty validator set")
	}
	if len(b.Signatures) == 0 {
		return errors.New("no signatures")
	}

	thresh := opts.MinSignatures
	if thresh == 0 {
		thresh = (2*len(b.ValidatorSet) + 2) / 3 // ceil(2N/3)
	}

	// Build a key-hash → entry index for validator lookup.
	idx := make(map[[32]byte]int, len(b.ValidatorSet))
	for i, v := range b.ValidatorSet {
		idx[v.PublicKeyHash] = i
	}

	canonical := b.CanonicalHash()

	seen := make(map[[32]byte]bool, len(b.Signatures))
	for _, sig := range b.Signatures {
		i, ok := idx[sig.PublicKeyHash]
		if !ok {
			// Signature from a non-validator — ignore (not an error,
			// just not counted).
			continue
		}
		if seen[sig.PublicKeyHash] {
			continue // duplicate
		}

		// Cryptographic verification.
		v := b.ValidatorSet[i]
		if !verifySig(v, sig.Signature, canonical) {
			// Skip without erroring; threshold rule decides.
			continue
		}

		seen[sig.PublicKeyHash] = true
	}

	if len(seen) < thresh {
		return fmt.Errorf("%w: %d/%d verified, need %d",
			ErrInsufficientSignatures, len(seen), len(b.ValidatorSet), thresh)
	}
	return nil
}

// verifySig dispatches to the right cryptographic verifier based on
// the validator's signature type. Returns false on type mismatch or
// signature failure.
func verifySig(v ValidatorEntry, sig []byte, hash [32]byte) bool {
	switch v.Type {
	case protocol.SignatureTypeED25519:
		if len(v.PublicKey) != ed25519.PublicKeySize {
			return false
		}
		return ed25519.Verify(ed25519.PublicKey(v.PublicKey), hash[:], sig)
	default:
		// Other signature types not yet supported by the bundle path.
		return false
	}
}

// VerifyOptions configures bundle verification.
type VerifyOptions struct {
	// MinSignatures overrides the default 2/3-of-validator-set threshold.
	// Zero means use the default.
	MinSignatures int
}
