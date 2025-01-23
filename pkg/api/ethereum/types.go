// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package ethrpc

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

type Number big.Int
type Bytes []byte
type Bytes32 [32]byte
type Address [20]byte

func NewNumber(v int64) *Number { return (*Number)(big.NewInt(v)) }

func (n *Number) Int() *big.Int { return (*big.Int)(n) }

func (n *Number) Equal(m *Number) bool {
	return n.Int().Cmp(m.Int()) == 0
}

func (n *Number) MarshalJSON() ([]byte, error) {
	s := n.Int().Text(16)
	return json.Marshal("0x" + s)
}

func (n *Number) UnmarshalJSON(c []byte) error {
	var s string
	err := json.Unmarshal(c, &s)
	if err != nil {
		return err
	}

	s = strings.TrimPrefix(s, "0x")
	_, ok := n.Int().SetString(s, 16)
	if !ok {
		return fmt.Errorf("%q is not a valid number", s)
	}
	return nil
}

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
	return json.Marshal(fmt.Sprintf("0x%x", *b))
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
	return json.Marshal(fmt.Sprintf("0x%x", *b))
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
