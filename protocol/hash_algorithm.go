package protocol

import (
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

func (ka HashAlgorithm) String() string {
	switch ka {
	case SHA256:
		return "SHA256"
	case SHA256D:
		return "SHA256D"
	default:
		return fmt.Sprintf("HashAlgorithm:%d", ka)
	}
}

func (ka HashAlgorithm) BinarySize() int {
	return 1
}

func (ka HashAlgorithm) MarshalBinary() ([]byte, error) {
	return []byte{byte(ka)}, nil
}

func (ka *HashAlgorithm) UnmarshalBinary(b []byte) error {
	if len(b) == 0 {
		return ErrNotEnoughData
	}
	*ka = HashAlgorithm(b[0])
	return nil
}

func (ka HashAlgorithm) MarshalJSON() ([]byte, error) {
	return json.Marshal(ka.String())
}

func (ka *HashAlgorithm) UnmarshalJSON(b []byte) error {
	var s *string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return nil
	}
	if s == nil {
		*ka = Unhashed
		return nil
	}

	*ka = HashAlgorithmByName(*s)
	if *ka == UnknownHashAlgorithm {
		return fmt.Errorf("invalid Hash algorithm: %q", *s)
	}
	return nil
}
