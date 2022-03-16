package database

import (
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (s *SignatureSet) Add(newSignatures ...protocol.Signature) int {
	if len(newSignatures) == 0 {
		return len(s.Signatures)
	}

	// Only keep one signature per public key
	seen := map[[32]byte]bool{}
	for _, sig := range s.Signatures {
		hash := sha256.Sum256(sig.GetPublicKey())
		seen[hash] = true
	}
	for _, sig := range newSignatures {
		hash := sha256.Sum256(sig.GetPublicKey())
		if seen[hash] {
			continue
		}
		seen[hash] = true
		s.Signatures = append(s.Signatures, sig)
	}

	return len(s.Signatures)
}
