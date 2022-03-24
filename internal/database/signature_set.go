package database

import (
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding/hash"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (s *SignatureSet) Add(newSignatures ...protocol.Signature) int {
	if len(newSignatures) == 0 {
		return len(s.Signatures)
	}

	// Only keep one signature per public key
	seen := map[[32]byte]bool{}
	for _, sig := range s.Signatures {
		hash := hashSignatureForUniqueness(sig)
		seen[hash] = true
	}
	for _, sig := range newSignatures {
		hash := hashSignatureForUniqueness(sig)
		if seen[hash] {
			continue
		}
		seen[hash] = true
		s.Signatures = append(s.Signatures, sig)
	}

	return len(s.Signatures)
}

// hashSignatureForUniqueness returns a hash that is used to prevent two
// signatures from the same key.
func hashSignatureForUniqueness(sig protocol.Signature) [32]byte {
	switch sig := sig.(type) {
	case *protocol.SyntheticSignature:
		// Multiple synthetic signatures doesn't make sense, but if they're
		// unique... ok
		hasher := make(hash.Hasher, 0, 3)
		hasher.AddUrl(sig.SourceNetwork)
		hasher.AddUrl(sig.DestinationNetwork)
		hasher.AddUint(sig.SequenceNumber)
		return *(*[32]byte)(hasher.MerkleHash())

	case *protocol.ReceiptSignature:
		// Multiple receipts doesn't make sense, but if they anchor to a unique
		// root... ok
		return *(*[32]byte)(sig.Result)

	case *protocol.InternalSignature:
		// Internal signatures only make any kind of sense if they're coming
		// from the local network, so they should never be different
		return sha256.Sum256([]byte("Accumulate Internal Signature"))

	default:
		// Normal signatures must come from a unique key
		key := sig.GetPublicKey()
		if key == nil {
			panic("attempted to add a signature that doesn't have a key!")
		}
		return sha256.Sum256(key)
	}
}
