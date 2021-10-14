package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

type KeyAlgorithm uint8

const (
	UnknownKeyAlgorithm KeyAlgorithm = iota
	RSA
	ECDSA
	ED25519
)

func KeyAlgorithmByName(s string) KeyAlgorithm {
	switch strings.ToUpper(s) {
	case "RSA":
		return RSA
	case "ECDSA":
		return ECDSA
	case "ED25519":
		return ED25519
	default:
		return UnknownKeyAlgorithm
	}
}

func (ka KeyAlgorithm) String() string {
	switch ka {
	case RSA:
		return "RSA"
	case ECDSA:
		return "ECDSA"
	case ED25519:
		return "ED25519"
	default:
		return fmt.Sprintf("KeyAlgorithm:%d", ka)
	}
}

func (ka KeyAlgorithm) BinarySize() int {
	return 1
}

func (ka KeyAlgorithm) MarshalBinary() ([]byte, error) {
	return []byte{byte(ka)}, nil
}

func (ka *KeyAlgorithm) UnmarshalBinary(b []byte) error {
	if len(b) == 0 {
		return ErrNotEnoughData
	}
	*ka = KeyAlgorithm(b[0])
	return nil
}

func (ka KeyAlgorithm) MarshalJSON() ([]byte, error) {
	return json.Marshal(ka.String())
}

func (ka *KeyAlgorithm) UnmarshalJSON(b []byte) error {
	var s *string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return nil
	}
	if s == nil {
		*ka = UnknownKeyAlgorithm
		return nil
	}

	*ka = KeyAlgorithmByName(*s)
	if *ka == UnknownKeyAlgorithm {
		return fmt.Errorf("invalid key algorithm: %q", *s)
	}
	return nil
}
