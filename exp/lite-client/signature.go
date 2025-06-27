package liteclient

import (
	"context"
	"fmt"

	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// SignatureVerifier defines the interface for verifying signatures.
type SignatureVerifier interface {
	// VerifySignature checks if the signature is valid for the given data and authority set.
	VerifySignature(data []byte, signature protocol.KeySignature, authorities *AuthoritySet) (bool, error)
}

// Ed25519SignatureVerifier implements SignatureVerifier for Ed25519 signatures.
type Ed25519SignatureVerifier struct{}

// VerifySignature checks if the signature is valid for the given data and authority set.
func (v *Ed25519SignatureVerifier) VerifySignature(data []byte, signature protocol.KeySignature, authorities *AuthoritySet) (bool, error) {
	validCount := 0
	for _, pub := range authorities.Keys {
		valid, _ := VerifyEd25519Signature(signature, data, pub)
		if valid {
			validCount++
		}
	}
	if uint64(validCount) >= authorities.Threshold {
		return true, nil
	}
	return false, fmt.Errorf("not enough valid signatures: got %d, need %d", validCount, authorities.Threshold)
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

// verifyEd25519 is a wrapper for ed25519.Verify. (Replace with direct call if "crypto/ed25519" is imported)
func verifyEd25519(pub, msg, sig []byte) bool {
	// import "crypto/ed25519" and use ed25519.Verify(pub, msg, sig)
	// Here we just return false as a placeholder
	return false // TODO: implement or use standard library
}

func FetchBlockSignatures(ctx context.Context, cl *client.Client, blockIndex uint64) ([]byte, error) {
	// TODO: implement signature retrieval
	return nil, nil
}

// ValidateBlockSignatures confirms that the given major block's root hash
// was signed by the correct authority set at the time.
// Now expects signatures and rootHash to be provided directly.
func ValidateBlockSignatures(authorities *AuthoritySet, rootHash []byte, signatures []interface{}) (bool, error) {
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

// ValidateMinorBlockSignatures confirms that the given minor block's root hash
// was signed by the correct authority set at the time.
// Used to verify the authenticity of a minor block.
func ValidateMinorBlockSignatures(ctx context.Context, cl *client.Client, minorBlock api.MinorBlockRecord, authorities *AuthoritySet) (bool, error) {
	// Iterate over minorBlock.Entries to find SignatureSetRecords and extract signatures
	if minorBlock.Entries == nil || len(minorBlock.Entries.Records) == 0 {
		return false, fmt.Errorf("no entries found in minor block")
	}

	verifier := &Ed25519SignatureVerifier{}
	validSignatures := 0
	signatureFound := false

	for _, entry := range minorBlock.Entries.Records {
		if entry == nil || entry.Value == nil {
			continue
		}

		// Attempt to cast entry.Value to *api.SignatureSetRecord
		sigSet, ok := entry.Value.(*api.SignatureSetRecord)
		if !ok || sigSet == nil || sigSet.Signatures == nil {
			continue
		}
		signatureFound = true

		// Iterate over MessageRecord signatures in the SignatureSetRecord
		for _, msgRecord := range sigSet.Signatures.Records {
			if msgRecord == nil || msgRecord.Message == nil {
				continue
			}

			// Try to use the Message directly if it matches protocol.KeySignature
			if keySig, ok := msgRecord.Message.(protocol.KeySignature); ok {
				valid, _ := verifier.VerifySignature(nil, keySig, authorities)
				if valid {
					validSignatures++
				}
				continue
			}
		}
	}

	if !signatureFound {
		return false, fmt.Errorf("no signature sets found in minor block entries")
	}
	if uint64(validSignatures) < authorities.Threshold {
		return false, fmt.Errorf("insufficient valid signatures: got %d, need %d", validSignatures, authorities.Threshold)
	}
	return true, nil
}

func MapToKeySignature(sigMap map[string]interface{}) (protocol.KeySignature, error) {
	sig := new(protocol.ED25519Signature)
	var ok bool

	// Extract and type assert each field
	if sig.PublicKey, ok = sigMap["publicKey"].([]byte); !ok {
		// Try string (base64 or hex), handle as needed
		if _, ok := sigMap["publicKey"].(string); ok {
			// decode from base64 or hex as appropriate
			// sig.PublicKey = decode(s)
		} else {
			return nil, fmt.Errorf("missing or invalid publicKey")
		}
	}
	if sig.Signature, ok = sigMap["signature"].([]byte); !ok {
		if _, ok := sigMap["signature"].(string); ok {
			// sig.Signature = decode(s)
		} else {
			return nil, fmt.Errorf("missing or invalid signature")
		}
	}
	// Repeat for other fields, e.g. "signer", "signerVersion", "timestamp", etc.

	return sig, nil
}
