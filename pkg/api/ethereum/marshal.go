package ethrpc

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

type Bytes []byte
type Bytes32 [32]byte
type Address [20]byte

func (b Bytes) MarshalJSON() ([]byte, error) {
	if len(b) == 0 {
		return json.Marshal("")
	}
	return json.Marshal(fmt.Sprintf("0x%x", b))
}

func (b *Bytes) UnmarshalJSON(c []byte) error {
	var s string
	err := json.Unmarshal(c, &s)
	if err != nil {
		return err
	}

	s = strings.TrimPrefix(s, "0x")
	*b, err = hex.DecodeString(s)
	return err
}

func (b *Bytes32) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("0x%x", b))
}

func (b *Bytes32) UnmarshalJSON(c []byte) error {
	var s string
	err := json.Unmarshal(c, &s)
	if err != nil {
		return err
	}

	s = strings.TrimPrefix(s, "0x")
	d, err := hex.DecodeString(s)
	if err != nil {
		return err
	}
	if len(d) != len(*b) {
		return fmt.Errorf("want %d bytes, got %d", len(*b), len(d))
	}

	*b = Bytes32(d)
	return nil
}

func (b *Address) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("0x%x", b))
}

func (b *Address) UnmarshalJSON(c []byte) error {
	var s string
	err := json.Unmarshal(c, &s)
	if err != nil {
		return err
	}

	s = strings.TrimPrefix(s, "0x")
	d, err := hex.DecodeString(s)
	if err != nil {
		return err
	}
	if len(d) != len(*b) {
		return fmt.Errorf("want %d bytes, got %d", len(*b), len(d))
	}

	*b = Address(d)
	return nil
}
