// MIT License
//
// Copyright 2018 Canonical Ledgers, LLC
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
// IN THE SOFTWARE.

package factom

import (
	"encoding/hex"
	"fmt"
)

// Bytes32 implements encoding.TextMarshaler and encoding.TextUnmarshaler to
// encode and decode hex strings with exactly 32 bytes of data, such as
// ChainIDs and KeyMRs.
type Bytes32 [32]byte

// Bytes implements encoding.TextMarshaler and encoding.TextUnmarshaler to
// encode and decode hex strings, such as an Entry's ExtIDs or Content.
type Bytes []byte

// NewBytes32 returns a Bytes32 populated with the data from s32, a hex encoded
// string.
func NewBytes32(s32 string) Bytes32 {
	var b32 Bytes32
	b32.Set(s32)
	return b32
}

// NewBytes returns a Bytes populated with the data from s, a hex encoded
// string.
func NewBytes(s string) Bytes {
	var b Bytes
	b.Set(s)
	return b
}

// Set decodes a hex string with exactly 32 bytes of data into b.
func (b *Bytes32) Set(hexStr string) error {
	return b.UnmarshalText([]byte(hexStr))
}

// Set decodes a hex string into b.
func (b *Bytes) Set(hexStr string) error {
	return b.UnmarshalText([]byte(hexStr))
}

// UnmarshalText decodes a hex string with exactly 32 bytes of data into b.
func (b *Bytes32) UnmarshalText(text []byte) error {
	if len(text) != hex.EncodedLen(len(b)) {
		return fmt.Errorf("invalid length")
	}
	if _, err := hex.Decode(b[:], text); err != nil {
		return err
	}
	return nil
}

// UnmarshalText decodes a hex string into b.
func (b *Bytes) UnmarshalText(text []byte) error {
	*b = make(Bytes, hex.DecodedLen(len(text)))
	if _, err := hex.Decode(*b, text); err != nil {
		return err
	}
	return nil
}

// String encodes b as a hex string.
func (b Bytes32) String() string {
	text, _ := b.MarshalText()
	return string(text)
}

// String encodes b as a hex string.
func (b Bytes) String() string {
	text, _ := b.MarshalText()
	return string(text)
}

// Type returns "Bytes32". Satisfies pflag.Value interface.
func (b Bytes32) Type() string {
	return "Bytes32"
}

// Type returns "Bytes". Satisfies pflag.Value interface.
func (b Bytes) Type() string {
	return "Bytes"
}

// MarshalText encodes b as a hex string. It never returns an error.
func (b Bytes32) MarshalText() ([]byte, error) {
	text := make([]byte, hex.EncodedLen(len(b)))
	hex.Encode(text, b[:])
	return text, nil
}

// MarshalText encodes b as a hex string. It never returns an error.
func (b Bytes) MarshalText() ([]byte, error) {
	text := make([]byte, hex.EncodedLen(len(b)))
	hex.Encode(text, b[:])
	return text, nil
}

// IsZero returns true if b is equal to its zero value.
func (b Bytes32) IsZero() bool {
	return b == Bytes32{}
}
