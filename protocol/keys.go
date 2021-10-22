package protocol

import (
	"bytes"
)

func (ks *SigSpec) FindKey(pubKey []byte) *KeySpec {
	// Check each key
	for _, candidate := range ks.Keys {
		// Try with each supported hash algorithm
		for _, ha := range []HashAlgorithm{Unhashed, SHA256, SHA256D} {
			if bytes.Equal(ha.MustApply(pubKey), candidate.PublicKey) {
				return candidate
			}
		}
	}

	return nil
}
