package liteclient

import (
	"crypto/ed25519"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ValidateMajorBlockSignature confirms that the given major block's root hash
// was signed by the correct authority set at the time.
// Used to verify the authenticity of a major block.
func ValidateBlockSignature(signature protocol.KeySignature, rootHash []byte, publicKey []byte) (bool, error) {
	valid, err := VerifySignature(signature, rootHash, publicKey)

	if err != nil {
		return false, fmt.Errorf("signature verification error: %v", err)
	}

	if !valid {
		return false, fmt.Errorf("invalid signature")
	}

	fmt.Printf("Signature verified: %x\n", signature.GetSignature())
	return true, nil
}

func VerifySignature(signature protocol.KeySignature, rootHash []byte, publicKey []byte) (bool, error) {
	sigBytes := signature.GetSignature()
	if len(sigBytes) != ed25519.SignatureSize {
		return false, fmt.Errorf("unexpected signature length: got %d, want %d", len(sigBytes), ed25519.SignatureSize)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return false, fmt.Errorf("invalid public key length: got %d, want %d", len(publicKey), ed25519.PublicKeySize)
	}

	valid := ed25519.Verify(publicKey, rootHash, sigBytes)
	return valid, nil
}
