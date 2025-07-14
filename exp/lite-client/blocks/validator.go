// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package blocks

import (
	"context"
	"crypto/ed25519"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// BlockValidator implements api.Validator to validate major block signatures.
type BlockValidator struct {
	// Authorities provides the set of trusted public keys for the network.
	Authorities AuthorityProvider
}

// NewBlockValidator creates a new BlockValidator.
func NewBlockValidator(provider AuthorityProvider) *BlockValidator {
	return &BlockValidator{
		Authorities: provider,
	}
}

// Validate checks the signatures of a major block anchor.
func (v *BlockValidator) Validate(ctx context.Context, envelope *messaging.Envelope, _ api.ValidateOptions) ([]*api.Submission, error) {
	var validSigners int

	// Get the current set of authorities and the voting threshold.
	authorities, threshold, err := v.Authorities.GetAuthorities(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get authorities: %w", err)
	}

	// The envelope's TxHash is the hash of the BlockAnchor message.
	hash := envelope.TxHash
	if hash == nil {
		return nil, fmt.Errorf("envelope is missing transaction hash")
	}

	for _, sig := range envelope.Signatures {
		ed, ok := sig.(*protocol.ED25519Signature)
		if !ok {
			// For now, we only support ED25519 signatures from validators.
			continue
		}

		// Check if the signer is a trusted authority.
		var key [32]byte
		kh := ed.GetPublicKeyHash()
		if len(kh) != 32 {
			continue
		}
		copy(key[:], kh)
		if !authorities[key] {
			continue
		}

		// Verify the signature directly against the transaction hash.
		// The signature is on the hash, not the message body.
		if !ed25519.Verify(ed.PublicKey, hash[:], ed.Signature) {
			continue
		}

		validSigners++
	}

	// Check if the number of valid signatures meets the threshold.
	if uint64(validSigners) < threshold {
		return nil, fmt.Errorf("not enough signatures: got %d, want %d", validSigners, threshold)
	}

	// If validation is successful, we return a nil submission list and no error.
	return nil, nil
}
