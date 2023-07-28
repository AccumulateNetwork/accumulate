// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package record

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// A Key is the key for a record.
type Key struct {
	values []any
}

func NewKey(v ...any) *Key {
	return &Key{v}
}

func (k *Key) Len() int {
	if k == nil {
		return 0
	}
	return len(k.values)
}

func (k *Key) Get(i int) any {
	if i < 0 || i >= k.Len() {
		return nil
	}
	return k.values[i]
}

func (k *Key) SliceI(i int) *Key {
	return &Key{k.values[i:]}
}

func (k *Key) SliceJ(j int) *Key {
	return &Key{k.values[:j]}
}

// Append creates a child key of this key.
func (k *Key) Append(v ...any) *Key {
	if len(v) == 0 {
		return k
	}
	if k.Len() == 0 {
		return &Key{v}
	}
	l := make([]any, len(k.values)+len(v))
	n := copy(l, k.values)
	copy(l[n:], v)
	return &Key{l}
}

// AppendKey appends one key to another. AppendKey will panic if K is not empty
// and L starts with a precomputed [KeyHash].
func (k *Key) AppendKey(l *Key) *Key {
	if k.Len() == 0 {
		return l
	}
	if l.Len() == 0 {
		return k
	}
	if _, ok := l.values[0].(KeyHash); ok {
		panic("cannot append a precomputed key hash to another key")
	}
	return k.Append(l.values...)
}

// Hash converts the record key to a storage key.
func (k *Key) Hash() KeyHash {
	if k.Len() == 0 {
		return KeyHash{}
	}

	// If the first value is a KeyHash, append to that
	if h, ok := k.values[0].(KeyHash); ok {
		return h.Append(k.values[1:]...)
	}
	return (KeyHash{}).Append(k.values...)
}

// String returns a human-readable string for the key.
func (k *Key) String() string {
	if k.Len() == 0 {
		return "()"
	}
	s := make([]string, len(k.values))
	for i, v := range k.values {
		switch v := v.(type) {
		case []byte:
			s[i] = hex.EncodeToString(v)
		case [32]byte:
			s[i] = hex.EncodeToString(v[:])
		case string:
			s[i] = v
		default:
			s[i] = fmt.Sprint(v)
		}
	}
	return strings.Join(s, ".")
}

func (k *Key) Stringf(format string) string {
	var args []any
	if k != nil {
		args = k.values
	}
	return fmt.Sprintf(format, args...)
}

// Copy returns a copy of the key.
func (k *Key) Copy() *Key {
	if k == nil {
		return nil
	}
	l := make([]any, len(k.values))
	copy(l, k.values)
	return &Key{l}
}

// CopyAsInterface implements [encoding.BinaryValue].
func (k *Key) CopyAsInterface() any {
	return k.Copy()
}

// Equal checks if the two keys are equal.
func (k *Key) Equal(l *Key) bool {
	// Must have a pointer receiver to work well with marshalling
	if k.Len() != l.Len() {
		return false
	}
	if k == nil || l == nil {
		return k.Len() == 0 && l.Len() == 0
	}
	for i := range k.values {
		if !keyPartsEqual(k.values[i], l.values[i]) {
			return false
		}
	}
	return true
}

// Compare compares two keys. Compare may panic if either key has an unexpected
// value type.
func (k *Key) Compare(l *Key) int {
	n := k.Len()
	if l.Len() < n {
		n = l.Len()
	}
	for i := 0; i < n; i++ {
		c := keyPartsCompare(k.values[i], l.values[i])
		if c != 0 {
			return c
		}
	}
	return k.Len() - l.Len()
}

// MarshalBinary marshals the key to bytes.
func (k *Key) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write the length
	_, _ = buf.Write(encoding.MarshalUint(uint64(k.Len())))
	if k.Len() == 0 {
		return buf.Bytes(), nil
	}

	// Write each field using the encoding writer, but prefix values with their
	// type code instead of with a field number. This is an abuse but 🤷 it
	// works.
	w := encoding.NewWriter(buf)
	for _, v := range k.values {
		p, err := asKeyPart(v)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		p.WriteBinary(w)
	}

	// Finish up
	_, _, err := w.Reset(nil)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary unmarshals a key from bytes.
func (k *Key) UnmarshalBinary(b []byte) error {
	return k.UnmarshalBinaryFrom(bytes.NewBuffer(b))
}

// UnmarshalBinaryFrom unmarshals a key from bytes.
func (k *Key) UnmarshalBinaryFrom(rd io.Reader) error {
	// We need a io.ByteScanner for binary.ReadUvarint
	br, ok := rd.(encoding.BytesReader)
	if !ok {
		br = bufio.NewReader(rd)
	}

	// Read the length
	n, err := binary.ReadUvarint(br)
	if err != nil {
		return errors.UnknownError.WithFormat("decode Key: %w", err)
	}

	// Allocate a key
	k.values = make([]any, n)

	// For each field, read the type code then read its value. The encoding
	// reader expects values to be prefixed with field numbers, and has certain
	// requirements for those field numbers, so this approach requires a certain
	// amount of hackiness. This is an abuse but 🤷 it works.
	r := encoding.NewReader(br)
	for i := range k.values {
		// Read the type code
		v, err := binary.ReadUvarint(br)
		if err != nil {
			return errors.UnknownError.WithFormat("decode Key: %w", err)
		}

		// Create a key part for that type code
		p, err := newKeyPart(typeCode(v))
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		// Read the value using the encoding reader
		p.ReadBinary(r)

		// Put the value in the key
		k.values[i] = p.Value()
	}

	// Finish up
	_, err = r.Reset(nil)
	if err != nil {
		return errors.UnknownError.WithFormat("decode Key: %w", err)
	}
	return nil
}

// MarshalJSON marshals the key to JSON.
//
// The key is marshaled as an list of single item objects where the key is the
// type code and the value is the value. For example:
//
//	[{"string": "Account"}, {"url": "foo.acme"}, {"string": "MainChain"}, {"string": "Element"}, {"int": 1}]
func (k *Key) MarshalJSON() ([]byte, error) {
	if k.Len() == 0 {
		return []byte("[]"), nil
	}

	parts := make([]map[string]any, len(k.values))
	for i, v := range k.values {
		// Convert the value to a key part
		p, err := asKeyPart(v)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

		// Record as { [type]: value }
		parts[i] = map[string]any{p.Type().String(): p}
	}
	return json.Marshal(parts)
}

// UnmarshalJSON unmarshals a key from JSON.
//
// See [Key.MarshalJSON] for the format and an example.
func (k *Key) UnmarshalJSON(b []byte) error {
	var parts []map[string]json.RawMessage
	err := json.Unmarshal(b, &parts)
	if err != nil {
		return err
	}

	k.values = make([]any, len(parts))
	for i, p := range parts {
		// Parts must be { [type]: value }
		if len(p) != 1 {
			return errors.EncodingError.WithFormat("invalid key part: expected { typ: val }, got %v", parts)
		}

		// Get the key and value
		var s string
		var b json.RawMessage
		for s, b = range p {
		}

		// Parse the type code
		typ, ok := typeCodeByName(s)
		if !ok {
			return errors.EncodingError.WithFormat("%q is not a valid type code", s)
		}

		// Create a key part
		kp, err := newKeyPart(typ)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		// Unmarshal the value
		err = json.Unmarshal(b, &kp)
		if err != nil {
			return errors.UnknownError.WithFormat("decode Key: %w", err)
		}

		// Put the value in the key
		k.values[i] = kp.Value()
	}
	return nil
}
