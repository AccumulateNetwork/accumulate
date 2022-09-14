package helpers

import "crypto/sha256"

func Hash(b []byte) []byte {
	if b == nil {
		return nil
	}
	h := sha256.Sum256(b)
	return h[:]
}
