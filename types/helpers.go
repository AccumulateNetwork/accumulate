// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
)

var _ encoding.Byter = (*Bytes)(nil)
var _ encoding.Byter = (*Bytes32)(nil)
var _ encoding.Byter = (*String)(nil)

type Bytes []byte

// MarshalJSON serializes ByteArray to hex
func (s *Bytes) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(fmt.Sprintf("%x", string(*s)))
	return b, err
}

// UnmarshalJSON serializes ByteArray to hex
func (s *Bytes) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)
	d, err := hex.DecodeString(str)
	if err != nil {
		return nil
	}
	*s = make([]byte, len(d))
	copy(*s, d)
	return err
}

func (s *Bytes) Bytes() []byte {
	return *s
}

func (s *Bytes) MarshalBinary() ([]byte, error) {
	var buf [8]byte
	l := s.Size(&buf)
	i := l - len(*s)
	data := make([]byte, l)
	copy(data, buf[:i])
	copy(data[i:], *s)
	return data, nil
}

func (s *Bytes) UnmarshalBinary(data []byte) (err error) {
	defer func() { //
		if recover() != nil { //
			err = fmt.Errorf("error unmarshaling byte array %v", err) //
		} //
	}()
	slen, l := binary.Uvarint(data)
	if l == 0 {
		return nil
	}
	if l < 0 {
		return fmt.Errorf("invalid data to unmarshal")
	}
	if len(data) < int(slen)+l {
		return fmt.Errorf("insufficient data to unmarshal")
	}
	*s = data[l : int(slen)+l]
	return err
}

func (s Bytes) AsBytes32() (ret Bytes32) {
	copy(ret[:], s)
	return ret
}

func (s *Bytes) Size(varintbuf *[8]byte) int {
	buf := varintbuf
	if buf == nil {
		buf = &[8]byte{}
	}
	l := uint64(len(*s))
	i := binary.PutUvarint(buf[:], l)

	return i + int(l)
}

// Bytes32 is a fixed array of 32 bytes
type Bytes32 [32]byte

// MarshalJSON serializes ByteArray to hex
func (s *Bytes32) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(s.ToString())
	return b, err
}

// UnmarshalJSON serializes ByteArray to hex
func (s *Bytes32) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)
	return s.FromString(str)
}

// Bytes returns the bite slice of the 32 byte array
func (s *Bytes32) Bytes() []byte {
	return s[:]
}

// FromBytes sets the byte array.
func (s *Bytes32) FromBytes(b []byte) error {
	if len(b) != 32 {
		return fmt.Errorf("expected 32 bytes string, received %d", len(b))
	}
	copy(s[:], b)
	return nil
}

// FromString takes a hex encoded string and sets the byte array.
// The input parameter, str, must be 64 hex characters in length
func (s *Bytes32) FromString(str string) error {
	if len(str) != 64 {
		return fmt.Errorf("expected 32 bytes string, received %s", str)
	}

	d, err := hex.DecodeString(str)
	if err != nil {
		return err
	}
	copy(s[:], d)
	return nil
}

// ToString will convert the 32 byte array into a hex string that is 64 hex characters in length
func (s *Bytes32) ToString() String {
	return String(hex.EncodeToString(s[:]))
}

func (s *Bytes32) AsByteArray() [32]byte {
	return *s
}

type String string

func (s *String) Bytes() []byte {
	return []byte(*s)
}

func (s *String) MarshalBinary() ([]byte, error) {
	b := Bytes(*s)
	return b.MarshalBinary()
}

func (s *String) UnmarshalBinary(data []byte) error {
	var b Bytes
	err := b.UnmarshalBinary(data)
	if err == nil {
		*s = String(b)
	}
	return err
}

func (s *String) Size(varintbuf *[8]byte) int {
	b := Bytes(*s)
	return b.Size(varintbuf)
}

func (s *String) AsString() *string {
	return (*string)(s)
}

func (s *String) MarshalJSON() ([]byte, error) {
	str := string(*s)
	b, err := json.Marshal(&str)
	return b, err
}

func (s *String) UnmarshalJSON(data []byte) error {
	*s = String(strings.Trim(string(data), `"`))
	return nil
}
