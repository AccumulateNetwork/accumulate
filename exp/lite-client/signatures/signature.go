package signatures

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	// api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Ed25519SignatureVerifier verifies Ed25519 signatures against an authority set.
type Ed25519SignatureVerifier struct{}

// VerifySignature checks if the signature is valid for the given data and authority set.
// Returns true if the signature is valid for any key in the authority set.
func (v *Ed25519SignatureVerifier) VerifySignature(data []byte, signature protocol.KeySignature, authorities *AuthoritySet) (bool, error) {
	for _, pub := range authorities.Keys {
		if len(pub) != ed25519.PublicKeySize {
			continue
		}
		if ed25519.Verify(pub, data, signature.GetSignature()) {
			return true, nil
		}
	}
	return false, fmt.Errorf("signature not valid for any authority key")
}

// VerifyEd25519Signature checks a KeySignature against a public key and data using ed25519.
func VerifyEd25519Signature(signature protocol.KeySignature, data []byte, publicKey []byte) (bool, error) {
	sigBytes := signature.GetSignature()
	if len(sigBytes) != 64 { // ed25519.SignatureSize
		return false, fmt.Errorf("unexpected signature length: got %d, want 64", len(sigBytes))
	}
	if len(publicKey) != 32 { // ed25519.PublicKeySize
		return false, fmt.Errorf("invalid public key length: got %d, want 32", len(publicKey))
	}
	return verifyEd25519(publicKey, data, sigBytes), nil
}

// verifyEd25519 uses the standard library to verify an Ed25519 signature.
func verifyEd25519(pub, msg, sig []byte) bool {
	return ed25519.Verify(pub, msg, sig)
}

// FetchBlockSignatures is a stub for retrieving block signatures. Implement as needed.
func FetchBlockSignatures(ctx context.Context, cl *client.Client, blockIndex uint64) ([]byte, error) {
	return nil, fmt.Errorf("FetchBlockSignatures not implemented")
}

// SignatureValidationPlan describes the high-level plan for validating signatures on blocks.
// These helpers may call lower-level logic in the blocks package.

// ValidateBlockSignatures confirms that the given major block's root hash
// was signed by the correct authority set at the time.
// Now expects signatures and rootHash to be provided directly.
func ValidateBlockSignatures(authorities *AuthoritySet, rootHash []byte, signatures []interface{}) (bool, error) {
	// Real implementation: verifies signatures using Ed25519SignatureVerifier
	if len(signatures) == 0 {
		return false, fmt.Errorf("no signatures found in major block")
	}

	verifier := &Ed25519SignatureVerifier{}
	validSignatures := 0
	for _, sigIface := range signatures {
		sigMap, ok := sigIface.(map[string]interface{})
		if !ok {
			continue
		}
		keySig, err := MapToKeySignature(sigMap)
		if err != nil {
			continue
		}
		valid, _ := verifier.VerifySignature(rootHash, keySig, authorities)
		if valid {
			validSignatures++
		}
	}
	if uint64(validSignatures) < authorities.Threshold {
		return false, fmt.Errorf("insufficient valid signatures: got %d, need %d", validSignatures, authorities.Threshold)
	}
	return true, nil
}

// ValidateMinorBlockSignatures is a stub for validating signatures on minor blocks.
// Calls lower-level logic in blocks package when implemented.
// ValidateMinorBlockSignatures is a stub for validating signatures on minor blocks.
func ValidateMinorBlockSignatures(ctx context.Context, cl *client.Client, minorBlock interface{}, authorities *AuthoritySet) (bool, error) {
	return false, fmt.Errorf("not implemented: ValidateMinorBlockSignatures")
}

// TODO: Add more signature validation helpers and stubs as needed.

// MapToKeySignature converts a map[string]interface{} to a protocol.KeySignature.
// Handles both []byte and string (base64 or hex) for publicKey and signature fields.
func MapToKeySignature(sigMap map[string]interface{}) (protocol.KeySignature, error) {
	sig := new(protocol.ED25519Signature)
	var ok bool

	// Helper to decode string as base64, then hex if base64 fails
	decodeString := func(s string, expectedLen int) ([]byte, error) {
		b, err := base64.StdEncoding.DecodeString(s)
		if err == nil && len(b) == expectedLen {
			return b, nil
		}
		b, err = hex.DecodeString(strings.TrimPrefix(s, "0x"))
		if err == nil && len(b) == expectedLen {
			return b, nil
		}
		return nil, fmt.Errorf("invalid encoding or length for field (wanted %d bytes)", expectedLen)
	}

	// PublicKey
	if sig.PublicKey, ok = sigMap["publicKey"].([]byte); !ok {
		if s, ok := sigMap["publicKey"].(string); ok {
			b, err := decodeString(s, ed25519.PublicKeySize)
			if err != nil {
				return nil, fmt.Errorf("invalid publicKey: %w", err)
			}
			sig.PublicKey = b
		} else {
			return nil, fmt.Errorf("missing or invalid publicKey")
		}
	}
	// Signature
	if sig.Signature, ok = sigMap["signature"].([]byte); !ok {
		if s, ok := sigMap["signature"].(string); ok {
			b, err := decodeString(s, ed25519.SignatureSize)
			if err != nil {
				return nil, fmt.Errorf("invalid signature: %w", err)
			}
			sig.Signature = b
		} else {
			return nil, fmt.Errorf("missing or invalid signature")
		}
	}
	// Optionally handle other fields (signer, version, timestamp) as needed
	return sig, nil
}
