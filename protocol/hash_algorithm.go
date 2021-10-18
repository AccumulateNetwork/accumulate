package protocol

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
)

type HashAlgorithm uint8

const (
	Unhashed HashAlgorithm = iota
	UnknownHashAlgorithm
	SHA256
	SHA256D
)

func HashAlgorithmByName(s string) HashAlgorithm {
	switch strings.ToUpper(s) {
	case "", "NONE", "RAW", "UNHASHED":
		return Unhashed
	case "SHA256":
		return SHA256
	case "SHA256D":
		return SHA256D
	default:
		return UnknownHashAlgorithm
	}
}

func (ha HashAlgorithm) String() string {
	switch ha {
	case SHA256:
		return "SHA256"
	case SHA256D:
		return "SHA256D"
	default:
		return fmt.Sprintf("HashAlgorithm:%d", ha)
	}
}

func (ha HashAlgorithm) Apply(b []byte) ([]byte, error) {
	switch ha {
	case Unhashed:
		return b, nil
	case SHA256:
		kh := sha256.Sum256(b)
		return kh[:], nil
	case SHA256D:
		kh := sha256.Sum256(b)
		kh = sha256.Sum256(kh[:])
		return kh[:], nil
	default:
		return nil, fmt.Errorf("invalid hash algorithm")
	}
}

func (ha HashAlgorithm) BinarySize() int {
	return 1
}

func (ha HashAlgorithm) MarshalBinary() ([]byte, error) {
	return []byte{byte(ha)}, nil
}

func (ha *HashAlgorithm) UnmarshalBinary(b []byte) error {
	if len(b) == 0 {
		return ErrNotEnoughData
	}
	*ha = HashAlgorithm(b[0])
	return nil
}

func (ha HashAlgorithm) MarshalJSON() ([]byte, error) {
	return json.Marshal(ha.String())
}

func (ha *HashAlgorithm) UnmarshalJSON(b []byte) error {
	var s *string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return nil
	}
	if s == nil {
		*ha = Unhashed
		return nil
	}

	*ha = HashAlgorithmByName(*s)
	if *ha == UnknownHashAlgorithm {
		return fmt.Errorf("invalid Hash algorithm: %q", *s)
	}
	return nil
}
