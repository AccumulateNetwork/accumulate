// Copyright 2025 The Accumulate Authors
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
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	binary2 "gitlab.com/accumulatenetwork/core/schema/pkg/binary"
	json2 "gitlab.com/accumulatenetwork/core/schema/pkg/json"
)

// AI: Key represents a hierarchical or composite key for database records in
// AI: Accumulate. The first element of the key is typically a type or namespace
// AI: (e.g., "Account", "Transaction") or a KeyHash (for long account URLs).
// AI: Keys can be extended using Append or AppendKey to create more specific
// AI: record paths. This structure supports both human-readable and hashed keys.
// AI:
// AI: Examples:
// AI:   // Short account URL (used directly):
// AI:   Key{"Account", "alice.acme"}
// AI:
// AI:   // Long account URL (hashed):
// AI:   Key{KeyHash(0x1234...), "Url"}
// AI:   // The hash maps to the original URL via a special mapping in the db.
// AI:
// AI:   // Extended key (e.g., for a field):
// AI:   Key{"Account", "alice.acme", "MainChain"}
// AI:
// AI: The Key type enables structured, efficient, and extensible record lookup.
type Key struct {
	values []any
	hash   *KeyHash
}

func NewKey(v ...any) *Key {
	return (*Key)(nil).Append(v...)
}

func KeyFromHash(kh KeyHash) *Key {
	return NewKey(kh)
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
	return &Key{values: k.values[i:]}
}

func (k *Key) SliceJ(j int) *Key {
	return &Key{values: k.values[:j]}
}

// Append creates a child key of this key.
func (k *Key) Append(v ...any) *Key {
	if len(v) == 0 {
		return k
	}
	if k.Len() == 0 {
		return &Key{values: v}
	}
	l := make([]any, len(k.values)+len(v))
	n := copy(l, k.values)
	copy(l[n:], v)
	return &Key{values: l}
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
	if k.hash != nil {
		return *k.hash
	}

	// If the first value is a KeyHash, append to that
	var kh KeyHash
	if h, ok := k.values[0].(KeyHash); ok {
		kh = h.Append(k.values[1:]...)
	} else {
		kh = (KeyHash{}).Append(k.values...)
	}
	k.hash = &kh
	return kh
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
	return &Key{values: l}
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
	// The pools are causing difficult to diagnose concurrency bugs, so we're
	// going to avoid them for now (though it hurts performance)
	buf := new(bytes.Buffer)
	enc := binary2.NewEncoder(buf)

	// buf := bufferPool.Get()
	// defer bufferPool.Put(buf)

	// enc := binEncPool.Get(buf, binary2.WithBufferPool(bufferPool))
	// defer binEncPool.Put(enc)

	err := k.MarshalBinaryV2(enc)
	return buf.Bytes(), err
}

// UnmarshalBinary unmarshals a key from bytes.
func (k *Key) UnmarshalBinary(b []byte) error {
	return k.UnmarshalBinaryFrom(bytes.NewBuffer(b))
}

func (k *Key) MarshalBinaryV2(enc *binary2.Encoder) error {
	err := enc.StartObject()
	if err != nil {
		return errors.UnknownError.WithFormat("encode Key: %w", err)
	}

	enc.WithOptions(binary2.IgnoreFieldOrder())

	// Write the key length (not prefixed with a field number)
	err = enc.NoField()
	if err != nil {
		return errors.UnknownError.WithFormat("encode Key: %w", err)
	}
	err = enc.EncodeUint(uint64(len(k.values)))
	if err != nil {
		return errors.UnknownError.WithFormat("encode Key: %w", err)
	}

	// Write each field using the encoding writer, but prefix values with their
	// type code instead of with a field number. This is an abuse but 🤷 it
	// works.
	for _, v := range k.values {
		v, typ, ok := normalize(v)
		if !ok {
			return invalidKeyPart(v)
		}
		err = enc.Field(uint(typ))
		if err != nil {
			return errors.UnknownError.WithFormat("encode Key: %w", err)
		}
		switch typ {
		case typeCodeUrl:
			err = enc.EncodeString(v.(*url.URL).String())
		case typeCodeTxid:
			err = enc.EncodeString(v.(*url.TxID).String())
		case typeCodeTime:
			err = enc.EncodeInt(v.(time.Time).UTC().Unix())
		default:
			err = enc.Encode(v)
		}
		if err != nil {
			return errors.UnknownError.WithFormat("encode Key: %w", err)
		}
	}

	err = enc.EndObject()
	if err != nil {
		return errors.UnknownError.WithFormat("encode Key: %w", err)
	}
	return nil
}

func (k *Key) UnmarshalBinaryV2(dec *binary2.Decoder) error {
	err := dec.StartObject()
	if err != nil {
		return errors.UnknownError.WithFormat("decode Key: %w", err)
	}

	dec.WithOptions(binary2.IgnoreFieldOrder())

	// Read the key length (not prefixed with a field number)
	err = dec.NoField()
	if err != nil {
		return errors.UnknownError.WithFormat("decode Key: %w", err)
	}
	n, err := dec.DecodeUint()
	if err != nil {
		return errors.UnknownError.WithFormat("decode Key: %w", err)
	}

	// Allocate the values list
	k.values = make([]any, n)

	// For each field, read the type code then read its value. The encoding
	// reader expects values to be prefixed with field numbers, and has certain
	// requirements for those field numbers, so this approach requires a certain
	// amount of hackiness. This is an abuse but 🤷 it works.
	for i := range k.values {
		// Read the type code
		v, err := dec.Field()
		if err != nil {
			return errors.UnknownError.WithFormat("decode Key: %w", err)
		}

		// Create a key part for that type code
		p, err := newKeyPart(typeCode(v))
		if err != nil {
			return errors.UnknownError.WithFormat("decode Key: %w", err)
		}

		// Read the value using the encoding reader
		err = p.ReadBinary2(dec)
		if err != nil {
			return errors.UnknownError.WithFormat("decode Key: %w", err)
		}

		// Put the value in the key
		k.values[i] = p.Value()
	}

	err = dec.EndObject()
	if err != nil {
		return errors.UnknownError.WithFormat("decode Key: %w", err)
	}
	return nil
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
	// The pools are causing difficult to diagnose concurrency bugs, so we're
	// going to avoid them for now (though it hurts performance)
	buf := new(bytes.Buffer)
	enc := json2.NewEncoder(buf)

	// buf := bufferPool.Get()
	// defer bufferPool.Put(buf)

	// enc := jsonEncPool.Get(buf)
	// defer jsonEncPool.Put(enc)

	err := k.MarshalJSONV2(enc)
	return buf.Bytes(), err
}

func (k *Key) MarshalJSONV2(enc *json2.Encoder) error {
	err := enc.StartArray()
	if err != nil {
		return err
	}

	for _, v := range k.values {
		v, typ, ok := normalize(v)
		if !ok {
			return invalidKeyPart(v)
		}
		err = errors.First(
			enc.StartObject(),
			enc.Field(typ.String()),
			enc.Encode(v),
			enc.EndObject(),
		)
		if err != nil {
			return err
		}
	}

	return enc.EndArray()
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
		err = json.Unmarshal(b, kp)
		if err != nil {
			return errors.UnknownError.WithFormat("decode Key: %w", err)
		}

		// Put the value in the key
		k.values[i] = kp.Value()
	}
	return nil
}
