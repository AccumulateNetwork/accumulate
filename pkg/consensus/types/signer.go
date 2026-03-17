// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types

import (
	"crypto/ed25519"
	"errors"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Signer is the interface for signing consensus messages.
// This abstraction allows different key management strategies while
// maintaining a consistent signing interface for the consensus layer.
type Signer interface {
	// Sign signs the given data and returns the signature.
	Sign(data []byte) ([]byte, error)

	// PublicKey returns the ed25519 public key used for signing.
	PublicKey() ed25519.PublicKey

	// ValidatorID returns the validator identity string.
	// For ADI-based signers, this is the ADI URL (e.g., "acc://validator-1.acme").
	// For raw key signers, this is typically the hex-encoded public key.
	ValidatorID() string
}

// ADISigner implements Signer using an ADI-based identity.
//
// DAG-BFT validators use the same key infrastructure as Accumulate transactions:
//   - Validator identity = ADI URL (e.g., acc://validator-1.acme)
//   - Signing key = Key from ADI's key page (e.g., acc://validator-1.acme/book/1)
//   - Certificate signing = Same as direct transaction signing
//
// This enables:
//   - No separate key management for consensus
//   - Key rotation by updating the key page
//   - Multi-sig support if needed
//   - Leveraging existing ADI permission system
type ADISigner struct {
	// KeyPageURL is the URL of the key page containing the signing key.
	// This identifies which key book and page the key is registered in.
	KeyPageURL *url.URL

	// PrivateKey is the ed25519 private key registered for direct signing
	// on the key page.
	PrivateKey ed25519.PrivateKey
}

// NewADISigner creates a new ADISigner with the given key page URL and private key.
// The key page URL should be in the format acc://identity/book/page (e.g., acc://validator-1.acme/book/1).
func NewADISigner(keyPageURL *url.URL, privateKey ed25519.PrivateKey) (*ADISigner, error) {
	if keyPageURL == nil {
		return nil, errors.New("key page URL is required")
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid private key size")
	}
	return &ADISigner{
		KeyPageURL: keyPageURL,
		PrivateKey: privateKey,
	}, nil
}

// Sign signs the given data using the private key from the key page.
func (s *ADISigner) Sign(data []byte) ([]byte, error) {
	return ed25519.Sign(s.PrivateKey, data), nil
}

// PublicKey returns the ed25519 public key derived from the private key.
func (s *ADISigner) PublicKey() ed25519.PublicKey {
	return s.PrivateKey.Public().(ed25519.PublicKey)
}

// ValidatorID returns the ADI URL as the validator identity.
// This returns the root identity from the key page URL (e.g., acc://validator-1.acme).
func (s *ADISigner) ValidatorID() string {
	return s.KeyPageURL.RootIdentity().String()
}

// RawKeySigner implements Signer using a raw ed25519 key pair.
// This is the legacy signing approach and is useful for testing
// or when ADI-based identity is not yet configured.
type RawKeySigner struct {
	privateKey ed25519.PrivateKey
}

// NewRawKeySigner creates a new RawKeySigner with the given private key.
func NewRawKeySigner(privateKey ed25519.PrivateKey) (*RawKeySigner, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid private key size")
	}
	return &RawKeySigner{privateKey: privateKey}, nil
}

// Sign signs the given data using the raw private key.
func (s *RawKeySigner) Sign(data []byte) ([]byte, error) {
	return ed25519.Sign(s.privateKey, data), nil
}

// PublicKey returns the ed25519 public key.
func (s *RawKeySigner) PublicKey() ed25519.PublicKey {
	return s.privateKey.Public().(ed25519.PublicKey)
}

// ValidatorID returns the hex-encoded public key as the validator identity.
func (s *RawKeySigner) ValidatorID() string {
	return hexEncode(s.PublicKey())
}

// PrivateKey returns the underlying private key.
// This is provided for backward compatibility with code that needs direct key access.
func (s *RawKeySigner) PrivateKey() ed25519.PrivateKey {
	return s.privateKey
}
