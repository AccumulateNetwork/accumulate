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

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Header is what the walker needs to verify a block. It carries the
// fields the launcher consumes (StateTreeRoot for convergence, Height
// for indexing) plus the SequencedMessage that validators actually
// signed.
//
// Wire alignment with Accumulate's protocol (verified at
// internal/core/crosschain/anchoring.go:212–221 for the producer
// side and internal/core/execute/v2/block/msg_block_anchor.go:204–207
// for the verifier): validators sign sha256(MarshalBinary(seq))
// where seq is the SequencedMessage wrapping the anchor txn — NOT
// the anchor txn's own GetHash(). Header.Sequenced carries that
// SequencedMessage so CanonicalHash can return seq.Hash() directly,
// and HeaderSignature.Signable can point to it for protocol.
// KeySignature.Verify.
//
// Synthetic test fixtures that don't construct full SequencedMessage
// values leave Sequenced nil; CanonicalHash falls back to a fields
// hash so the raw-ed25519 test path keeps working. Production
// sources MUST populate Sequenced.
type Header struct {
	// Height is the partition's block index for this header
	// (PartitionAnchor.MinorBlockIndex for the source partition).
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

	// Sequenced is the SequencedMessage validators signed over.
	// Live sources populate this from a BlockAnchor message's
	// Anchor field; tests may leave it nil to use the fields-hash
	// fallback in CanonicalHash.
	Sequenced *messaging.SequencedMessage

	// AnchorTxHash is a transitional field from the first v2 draft
	// (which mistakenly used the anchor txn's GetHash() as the
	// signed value). Removed when phase 3 rewrites APISource to
	// populate Sequenced. Existing test fixtures that rely on it
	// continue to work via the CanonicalHash precedence rules.
	//
	// Deprecated: use Sequenced.
	AnchorTxHash [32]byte
}

// CanonicalHash returns the value validator signatures must be
// verified against. Precedence:
//
//	1. Sequenced != nil → seq.Hash() (live-source headers)
//	2. AnchorTxHash != zero → AnchorTxHash (transitional path
//	   from the first v2 draft; removed in phase 3)
//	3. otherwise → fields hash (synthetic test fixtures)
func (h *Header) CanonicalHash() [32]byte {
	if h.Sequenced != nil {
		return h.Sequenced.Hash()
	}
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

// HeaderSignature is one validator's signature on a header. Two
// carrier formats are supported:
//
//   - PublicKeyHash + Signature (raw): used by test fixtures and any
//     caller that has already extracted the bytes. Verification path
//     dispatches on Validator.Type and does direct ed25519 / etc
//     against the header's CanonicalHash.
//
//   - KeySignature + Signable: used by APISource and any caller
//     pulling signatures from the live network. KeySignature is the
//     validator's protocol-level signature object; Signable is the
//     thing it was made over (typically the SequencedMessage from
//     the BlockAnchor wire message — NOT the inner anchor txn).
//     Verification delegates to KeySignature.Verify(KeySignature,
//     Signable), which handles the full Accumulate signature
//     semantics. When KeySignature is set, the raw fields are
//     ignored.
type HeaderSignature struct {
	PublicKeyHash [32]byte
	Signature     []byte

	// KeySignature, if non-nil, supersedes PublicKeyHash + Signature.
	KeySignature protocol.KeySignature

	// Signable is the value KeySignature was made over. Required
	// when KeySignature is set; nil falls back to using the Header
	// itself (which only works if Sequenced is set on the Header,
	// or the test fixture is using fields-hash semantics).
	Signable protocol.Signable
}

// Hash satisfies protocol.Signable so KeySignature.Verify can take
// *Header directly when no explicit Signable is supplied (test path).
// Returns the canonical hash (seq.Hash() for live-source headers).
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
//
// For the protocol-aware path: KeySignature.Verify takes a Signable.
// If HeaderSignature.Signable is set (live path: the SequencedMessage
// from the BlockAnchor), use it. Otherwise fall back to the Header
// itself, which only verifies cleanly when h.Sequenced is set
// (CanonicalHash returns seq.Hash()).
func verifySig(v Validator, s HeaderSignature, h *Header) bool {
	if s.KeySignature != nil {
		var target protocol.Signable = h
		if s.Signable != nil {
			target = s.Signable
		}
		return s.KeySignature.Verify(s.KeySignature, target)
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
