package protocol

import (
	"bytes"
	"fmt"
)

func (ms *KeyPage) FindKey(pubKey []byte) *KeySpec {
	// Check each key
	for _, candidate := range ms.Keys {
		// Try with each supported hash algorithm
		for _, ha := range []HashAlgorithm{Unhashed, SHA256, SHA256D} {
			if bytes.Equal(ha.MustApply(pubKey), candidate.PublicKey) {
				return candidate
			}
		}
	}

	return nil
}

// GetMofN
// return the signature requirements of the Key Page.  Each Key Page requires
// m of n signatures, where m <= n, and n is the number of keys on the key page.
// m is the Threshold number of signatures required to validate a transaction
func (ms *KeyPage) GetMofN() (m, n uint64) {
	m = ms.Threshold
	n = uint64(len(ms.Keys))
	return m, n
}

// SetThreshold
// set the signature threshold to M.  Returns an error if m > n
func (ms *KeyPage) SetThreshold(m uint64) error {
	if m <= uint64(len(ms.Keys)) {
		ms.Threshold = m
	} else {
		return fmt.Errorf("cannot require %d signatures on a key page with %d keys", m, len(ms.Keys))
	}
}
