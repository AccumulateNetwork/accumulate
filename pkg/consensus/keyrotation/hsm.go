// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package keyrotation

import (
	"context"
	"crypto/ed25519"
	"fmt"
)

// HSMProvider is an interface for Hardware Security Module providers.
type HSMProvider interface {
	// GenerateKey generates a new key in the HSM and returns its public key.
	GenerateKey(ctx context.Context, keyID string) (ed25519.PublicKey, error)

	// Sign signs a message using the specified key in the HSM.
	Sign(ctx context.Context, keyID string, message []byte) ([]byte, error)

	// GetPublicKey retrieves the public key for a given key ID.
	GetPublicKey(ctx context.Context, keyID string) (ed25519.PublicKey, error)
}

// NewHSMProvider creates an HSM provider based on the configuration.
func NewHSMProvider(config HSMConfig) (HSMProvider, error) {
	switch config.Provider {
	case "aws-cloudhsm":
		return nil, fmt.Errorf("AWS CloudHSM not yet implemented")
	case "google-cloud-kms":
		return nil, fmt.Errorf("Google Cloud KMS not yet implemented")
	case "vault":
		return nil, fmt.Errorf("HashiCorp Vault not yet implemented")
	case "none", "":
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown HSM provider: %s", config.Provider)
	}
}
