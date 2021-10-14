package protocol

import (
	"bytes"
	"fmt"
)

func (ks *SigSpec) FindKey(pubKey []byte, ka KeyAlgorithm) (*KeySpec, error) {
	for _, key := range ks.Keys {
		if key.KeyAlgorithm != ka {
			continue
		}

		sigKH, err := key.HashAlgorithm.Apply(pubKey)
		if err != nil {
			return nil, fmt.Errorf("key spec: %v", err)
		}

		if bytes.Equal(key.PublicKey, sigKH) {
			return key, nil
		}
	}

	return nil, nil
}
