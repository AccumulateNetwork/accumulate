package liteclient

import (
	"context"
	"fmt"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ValidateMajorBlockSignature confirms that the given major block's root hash
// was signed by the correct authority set at the time.
// Used to verify the authenticity of a major block.
func ValidateBlockSignatures(ctx context.Context, cl *client.Client, blockIndex uint64, authorities *AuthoritySet, rootHash []byte) (bool, error) {
	// Query the specific major block using the block.go helper
	blocks, err := QueryMajorBlocks(ctx, cl, blockIndex, 1)
	if err != nil {
		return false, fmt.Errorf("failed to query major block: %v", err)
	}
	if len(blocks) == 0 {
		return false, fmt.Errorf("major block not found")
	}
	block := blocks[0]

	// Extract signatures
	sigsRaw, ok := block["signatures"]
	if !ok {
		return false, fmt.Errorf("no signatures found in block")
	}
	sigs, ok := sigsRaw.([]interface{})
	if !ok {
		return false, fmt.Errorf("invalid signatures format")
	}

	// Verify signatures against the authority set
	validSignatures := 0
	for _, sigIface := range sigs {
		sigMap, ok := sigIface.(map[string]interface{})
		if !ok {
			continue
		}
		// Convert sigMap to protocol.KeySignature as needed
		// This step depends on your actual signature unmarshaling logic
		// For illustration, let's assume you have a helper:
		keySig, err := MapToKeySignature(sigMap)
		if err != nil {
			continue
		}
		for _, pub := range authorities.Keys {
			valid, _ := verifyEd25519Signature(keySig, rootHash, pub)
			if valid {
				validSignatures++
				break
			}
		}
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
