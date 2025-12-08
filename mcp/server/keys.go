// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package server

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// KeyManager manages signing keys for transaction building
type KeyManager struct {
	mu   sync.RWMutex
	keys map[string]ed25519.PrivateKey // keyed by public key hash hex
}

// NewKeyManager creates a new key manager
func NewKeyManager() *KeyManager {
	return &KeyManager{
		keys: make(map[string]ed25519.PrivateKey),
	}
}

// GenerateKey generates a new ED25519 key pair and stores it
func (km *KeyManager) GenerateKey() (publicKey []byte, publicKeyHash []byte, err error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}

	hash := sha256.Sum256(pub)
	hashHex := hex.EncodeToString(hash[:])

	km.mu.Lock()
	km.keys[hashHex] = priv
	km.mu.Unlock()

	return pub, hash[:], nil
}

// ImportKey imports an existing private key
func (km *KeyManager) ImportKey(privateKeyHex string) (publicKey []byte, publicKeyHash []byte, err error) {
	privBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid private key hex: %w", err)
	}

	if len(privBytes) != ed25519.PrivateKeySize {
		return nil, nil, fmt.Errorf("invalid private key length: expected %d, got %d", ed25519.PrivateKeySize, len(privBytes))
	}

	priv := ed25519.PrivateKey(privBytes)
	pub := priv.Public().(ed25519.PublicKey)
	hash := sha256.Sum256(pub)
	hashHex := hex.EncodeToString(hash[:])

	km.mu.Lock()
	km.keys[hashHex] = priv
	km.mu.Unlock()

	return pub, hash[:], nil
}

// GetPublicKey returns the public key for a given public key hash
func (km *KeyManager) GetPublicKey(publicKeyHash []byte) ([]byte, error) {
	hashHex := hex.EncodeToString(publicKeyHash)

	km.mu.RLock()
	priv, ok := km.keys[hashHex]
	km.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("key not found: %s", hashHex)
	}

	return priv.Public().(ed25519.PublicKey), nil
}

// GetPrivateKey returns the private key for a given public key hash
func (km *KeyManager) GetPrivateKey(publicKeyHash []byte) ([]byte, error) {
	hashHex := hex.EncodeToString(publicKeyHash)

	km.mu.RLock()
	priv, ok := km.keys[hashHex]
	km.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("key not found: %s", hashHex)
	}

	return priv, nil
}

// Sign signs a message with the key identified by public key hash
func (km *KeyManager) Sign(publicKeyHash []byte, message []byte) ([]byte, error) {
	hashHex := hex.EncodeToString(publicKeyHash)

	km.mu.RLock()
	priv, ok := km.keys[hashHex]
	km.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("key not found: %s", hashHex)
	}

	return ed25519.Sign(priv, message), nil
}

// ListKeys returns all public key hashes
func (km *KeyManager) ListKeys() []string {
	km.mu.RLock()
	defer km.mu.RUnlock()

	keys := make([]string, 0, len(km.keys))
	for hash := range km.keys {
		keys = append(keys, hash)
	}
	return keys
}

// HasKey checks if a key exists
func (km *KeyManager) HasKey(publicKeyHash []byte) bool {
	hashHex := hex.EncodeToString(publicKeyHash)

	km.mu.RLock()
	_, ok := km.keys[hashHex]
	km.mu.RUnlock()

	return ok
}

// LiteAddress computes a lite address from a public key
func LiteAddress(publicKey []byte, tokenURL string) (*url.URL, error) {
	if tokenURL == "" {
		tokenURL = "acc://ACME"
	}

	token, err := url.Parse(tokenURL)
	if err != nil {
		return nil, fmt.Errorf("invalid token URL: %w", err)
	}

	return protocol.LiteTokenAddress(publicKey, token.String(), protocol.SignatureTypeED25519)
}

// LiteIdentity computes a lite identity URL from a public key
func LiteIdentity(publicKey []byte) *url.URL {
	return protocol.LiteAuthorityForKey(publicKey, protocol.SignatureTypeED25519)
}
