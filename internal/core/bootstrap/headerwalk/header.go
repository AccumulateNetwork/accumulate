// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package headerwalk is the v2 bootstrap trust phase. It walks block
// headers verifying the validator-quorum signature on each, producing
// a verified current-block StateTreeAnchor that the data phase's
// reconstructed BPT root must match for the launcher to promote to
// ACTIVE.
//
// The walker tracks operators-keybook state across blocks: when a
// block contains transactions that update the operators key book, the
// walker applies those deltas (no signature checking — the block's
// quorum signature is the protocol's attestation) so the next block's
// quorum is verified against the up-to-date set.
package headerwalk

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Header is what the walker needs to verify a block. It carries the
// fields the launcher consumes (StateTreeRoot for convergence, Height
// for indexing) plus the AnchorTxHash that validators actually signed.
//
// Wire alignment: in Accumulate's protocol, validators sign the hash
// of an anchor transaction (BlockValidatorAnchor / DirectoryAnchor)
// whose body embeds a PartitionAnchor. The PartitionAnchor's
// MinorBlockIndex / RootChainAnchor / StateTreeAnchor map onto our
// Height / ChainRoot / StateTreeRoot. The hash validators signed is
// the transaction hash, not a synthetic field-by-field hash — so
// CanonicalHash returns AnchorTxHash directly. A source that mints
// synthetic Headers (test fixtures) sets AnchorTxHash to whatever
// it wants signers to sign over.
type Header struct {
	// Height is the partition's minor block height for this header
	// (PartitionAnchor.MinorBlockIndex).
	Height uint64

	// Time is the block time as recorded by the network.
	Time time.Time

	// ChainRoot is the partition's root chain anchor at this block
	// (PartitionAnchor.RootChainAnchor).
	ChainRoot [32]byte

	// StateTreeRoot is the BPT root committed by this block
	// (PartitionAnchor.StateTreeAnchor). The data phase's locally
	// reconstructed BPT must equal this for convergence to succeed.
	StateTreeRoot [32]byte

	// AnchorTxHash is the hash of the anchor transaction that
	// validators signed. For synthetic test fixtures, set this to
	// the value signers signed over. For live sources, this is the
	// anchor txn's GetHash().
	AnchorTxHash [32]byte
}

// CanonicalHash returns the value validator signatures must be
// verified against. For Headers populated from a live source, this
// is the anchor transaction hash. For synthetic Headers, it's
// whatever the test fixture set as AnchorTxHash.
//
// If AnchorTxHash is the zero value (which would normally indicate a
// misconfigured source), we fall back to a deterministic hash over
// (Height, Time, ChainRoot, StateTreeRoot). The fallback is intended
// for early test code that hasn't been updated yet — production
// sources MUST populate AnchorTxHash.
func (h *Header) CanonicalHash() [32]byte {
	if h.AnchorTxHash != ([32]byte{}) {
		return h.AnchorTxHash
	}
	return h.fieldsHash()
}

func (h *Header) fieldsHash() [32]byte {
	hh := sha256.New()
	var buf [8]byte

	binary.BigEndian.PutUint64(buf[:], h.Height)
	hh.Write(buf[:])

	binary.BigEndian.PutUint64(buf[:], uint64(h.Time.Unix()))
	hh.Write(buf[:])

	hh.Write(h.ChainRoot[:])
	hh.Write(h.StateTreeRoot[:])

	var out [32]byte
	copy(out[:], hh.Sum(nil))
	return out
}

// Validator is one entry in the operators key book — a public key and
// the signature scheme to verify with.
type Validator struct {
	PublicKeyHash [32]byte
	PublicKey     []byte
	Type          protocol.SignatureType
}

// ValidatorSet is the operators key book at a particular height. It
// is a pure data carrier; transitions between sets happen via Apply
// in the walker, driven by operators-keybook deltas in block bodies.
type ValidatorSet struct {
	Validators []Validator
}

// HeaderSignature is one validator's signature on a header's
// canonical hash. Two carrier formats are supported:
//
//   - PublicKeyHash + Signature (raw): used by test fixtures and any
//     caller that has already extracted the bytes. Verification path
//     dispatches on Validator.Type and does direct ed25519 / etc.
//
//   - KeySignature: used by APISource and any caller pulling
//     signatures from the live network. Verification delegates to
//     the protocol's KeySignature.Verify, which handles the
//     full Accumulate signature semantics (init/transaction hashes,
//     versioned signers, signature scheme dispatch). When set, the
//     raw fields are ignored.
type HeaderSignature struct {
	PublicKeyHash [32]byte
	Signature     []byte

	// KeySignature, if non-nil, supersedes PublicKeyHash + Signature.
	// VerifyQuorum delegates to KeySignature.Verify against the
	// header's canonical hash.
	KeySignature protocol.KeySignature
}

// Hash satisfies protocol.Signable so KeySignature.Verify can take
// *Header directly. Returns the canonical hash (anchor txn hash for
// live-source headers).
func (h *Header) Hash() [32]byte {
	return h.CanonicalHash()
}

// QuorumOptions tunes the verification rule. Default rule is "at
// least ceil(2/3) of the validator set must sign with distinct,
// cryptographically valid signatures."
type QuorumOptions struct {
	// MinSignatures overrides the default 2/3 rule. Zero means use
	// the default. Useful for tests; production almost always wants
	// the default.
	MinSignatures int
}

// ErrInsufficientQuorum is the sentinel returned when a header has
// signatures but they don't meet the threshold.
var ErrInsufficientQuorum = errors.New("headerwalk: insufficient validator quorum on header")

// VerifyQuorum checks that header is signed by ≥ threshold distinct
// validators from set, with cryptographically valid signatures over
// the header's canonical hash.
//
// Signatures from non-validators are ignored (not an error — they
// don't count toward the threshold). Duplicate signatures from the
// same validator count once. Cryptographically invalid signatures
// don't count, but also don't cause a hard error: the threshold rule
// decides outcome.
func VerifyQuorum(h *Header, set ValidatorSet, sigs []HeaderSignature, opts QuorumOptions) error {
	if h == nil {
		return errors.New("headerwalk: nil header")
	}
	if len(set.Validators) == 0 {
		return errors.New("headerwalk: empty validator set")
	}

	thresh := opts.MinSignatures
	if thresh == 0 {
		thresh = (2*len(set.Validators) + 2) / 3 // ceil(2N/3)
	}

	// Index the validator set by public-key hash for O(1) lookup.
	idx := make(map[[32]byte]int, len(set.Validators))
	for i, v := range set.Validators {
		idx[v.PublicKeyHash] = i
	}

	seen := make(map[[32]byte]bool, len(sigs))
	for _, s := range sigs {
		// Resolve the public-key hash. KeySignature is authoritative
		// when present; raw-bytes path uses the explicit field.
		var pkh [32]byte
		if s.KeySignature != nil {
			copy(pkh[:], s.KeySignature.GetPublicKeyHash())
		} else {
			pkh = s.PublicKeyHash
		}

		i, ok := idx[pkh]
		if !ok {
			continue
		}
		if seen[pkh] {
			continue
		}
		v := set.Validators[i]
		if !verifySig(v, s, h) {
			continue
		}
		seen[pkh] = true
	}

	if len(seen) < thresh {
		return fmt.Errorf("%w: %d/%d verified, need %d at height %d",
			ErrInsufficientQuorum, len(seen), len(set.Validators), thresh, h.Height)
	}
	return nil
}

// verifySig dispatches: protocol-aware path when the signature
// carries a KeySignature (live network), raw-bytes path otherwise
// (test fixtures). Unsupported signature types return false, leaving
// the threshold rule to decide outcome.
func verifySig(v Validator, s HeaderSignature, h *Header) bool {
	if s.KeySignature != nil {
		// Protocol's Verify dispatches on the KeySignature's
		// concrete type and handles full Accumulate signature
		// semantics (versioned signers, init/transaction hashes,
		// signature scheme dispatch).
		return s.KeySignature.Verify(s.KeySignature, h)
	}

	// Raw-bytes path: caller supplied (PublicKeyHash, Signature)
	// directly. Used by test fixtures that don't construct full
	// protocol.KeySignature objects.
	switch v.Type {
	case protocol.SignatureTypeED25519:
		if len(v.PublicKey) != ed25519.PublicKeySize {
			return false
		}
		canonical := h.CanonicalHash()
		return ed25519.Verify(ed25519.PublicKey(v.PublicKey), canonical[:], s.Signature)
	default:
		return false
	}
}
