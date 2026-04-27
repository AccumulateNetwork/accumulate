// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package trustbundle

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// CanonicalHash returns a deterministic hash over the bundle's
// content, excluding the Signatures slice. Signers compute this hash
// and sign it; verifiers recompute the same hash and check signatures
// against it.
//
// The encoding is intentionally simple: each field is written in a
// fixed order with explicit length prefixes for variable-length data.
// Field ordering is part of the wire contract — changing it would
// break verifiers, so any future schema additions must append rather
// than reorder.
func (b *Bundle) CanonicalHash() [32]byte {
	h := sha256.New()
	w := func(p []byte) { h.Write(p) }
	wu64 := func(v uint64) {
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], v)
		w(buf[:])
	}
	wu32 := func(v uint32) {
		var buf [4]byte
		binary.BigEndian.PutUint32(buf[:], v)
		w(buf[:])
	}
	wstr := func(s string) {
		wu64(uint64(len(s)))
		w([]byte(s))
	}

	wu32(b.Version)
	wstr(b.Network)
	wstr(b.Partition)
	wu64(b.MajorBlockIndex)
	wu64(b.MinorBlockIndex)
	wu64(uint64(b.MajorBlockTimeUnix))

	// Per-partition anchors. Sort by partition name for canonical order.
	anchors := append([]PartitionAnchorEntry(nil), b.PerPartitionAnchors...)
	sort.Slice(anchors, func(i, j int) bool { return anchors[i].Partition < anchors[j].Partition })
	wu64(uint64(len(anchors)))
	for _, a := range anchors {
		wstr(a.Partition)
		w(a.RootChainAnchor[:])
		w(a.StateTreeAnchor[:])
	}

	// Minimum bootstrap set. Sort by URL for canonical order.
	mbs := append([]AccountEntry(nil), b.MinimumBootstrapSet...)
	sort.Slice(mbs, func(i, j int) bool {
		ui := ""
		uj := ""
		if mbs[i].Url != nil {
			ui = mbs[i].Url.String()
		}
		if mbs[j].Url != nil {
			uj = mbs[j].Url.String()
		}
		return ui < uj
	})
	wu64(uint64(len(mbs)))
	for _, e := range mbs {
		if e.Url != nil {
			wstr(e.Url.String())
		} else {
			wstr("")
		}
		wu64(uint64(len(e.State)))
		w(e.State)
		w(e.ValueHash[:])
	}

	// Validator set. Sort by public-key hash for canonical order.
	vs := append([]ValidatorEntry(nil), b.ValidatorSet...)
	sort.Slice(vs, func(i, j int) bool {
		for k := range vs[i].PublicKeyHash {
			if vs[i].PublicKeyHash[k] != vs[j].PublicKeyHash[k] {
				return vs[i].PublicKeyHash[k] < vs[j].PublicKeyHash[k]
			}
		}
		return false
	})
	wu64(uint64(len(vs)))
	for _, v := range vs {
		w(v.PublicKeyHash[:])
		wu64(uint64(len(v.PublicKey)))
		w(v.PublicKey)
		wu32(uint32(v.Type))
	}

	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// Signer abstracts the signing operation so tests can use ed25519
// directly while production uses protocol.SignWith*-style helpers.
type Signer interface {
	// Sign returns a raw signature over the canonical hash, plus the
	// validator's public-key hash and signature type.
	Sign(canonicalHash [32]byte) (sig []byte, pubKeyHash [32]byte, sigType protocol.SignatureType, err error)
}

// SignerFor builds a Signer from an ed25519 keypair. The validator's
// public-key hash is the sha256 of the public key (Accumulate's
// standard mapping for ED25519 validators).
func SignerFor(privateKey, publicKey []byte) Signer {
	return ed25519Signer{priv: privateKey, pub: publicKey}
}

type ed25519Signer struct {
	priv []byte
	pub  []byte
}

func (s ed25519Signer) Sign(canonicalHash [32]byte) ([]byte, [32]byte, protocol.SignatureType, error) {
	if len(s.priv) != ed25519.PrivateKeySize || len(s.pub) != ed25519.PublicKeySize {
		return nil, [32]byte{}, 0, fmt.Errorf("invalid ed25519 keypair (priv=%d pub=%d)", len(s.priv), len(s.pub))
	}
	pubHash := sha256.Sum256(s.pub)
	sig := ed25519.Sign(ed25519.PrivateKey(s.priv), canonicalHash[:])
	return sig, pubHash, protocol.SignatureTypeED25519, nil
}

// AddSignature signs the bundle's canonical hash with the supplied
// Signer and appends the signature to b.Signatures. Used by validators
// during the producer flow (#3974). Returns the signature for record-
// keeping (the aggregator can collect sigs across validators before
// the bundle is published).
func (b *Bundle) AddSignature(signer Signer) (ValidatorSignature, error) {
	hash := b.CanonicalHash()
	sig, pubHash, _, err := signer.Sign(hash)
	if err != nil {
		return ValidatorSignature{}, fmt.Errorf("sign: %w", err)
	}
	entry := ValidatorSignature{PublicKeyHash: pubHash, Signature: sig}
	b.Signatures = append(b.Signatures, entry)
	return entry, nil
}
