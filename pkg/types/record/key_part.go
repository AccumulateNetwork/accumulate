// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package record

import (
	"encoding/hex"
	"encoding/json"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	enc "gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// keyPart is a part of a [Key]. keyPart is used for marshalling.
type keyPart interface {
	// Type returns the key part's type.
	Type() typeCode

	// Value returns the key part's value.
	Value() any

	// WriteBinary writes the key part to the binary writer.
	//
	// The intended use of the binary writer is writing field-number tagged
	// struct field values. In this case we are abusing the binary writer by
	// treating the type code as if it was a field number. This is a violation
	// of the field-number tagged encoding scheme, but the writer doesn't pay
	// any attention to the field numbers so this still works.
	WriteBinary(w *enc.Writer)

	// ReadBinary reads the key part from the binary reader.
	//
	// The intended use of the binary reader is reading field-number tagged
	// struct field values. The binary reader _does_ pay attention to field
	// numbers, and regardless we don't know the 'field numbers' ahead of time
	// (since they are actually type codes) so we have to circumvent the field
	// number logic. [Key.UnmarshalBinaryFrom] handles reading the type codes,
	// so here we tell the binary reader we want to read field zero, which
	// instructs it to skip the field number logic and read the value directly.
	ReadBinary(r *enc.Reader)
}

// These methods are here to keep them close to the explanation above.

func (k intKeyPart) WriteBinary(w *enc.Writer)    { w.WriteInt(uint(k.Type()), int64(k)) }
func (k uintKeyPart) WriteBinary(w *enc.Writer)   { w.WriteUint(uint(k.Type()), uint64(k)) }
func (k stringKeyPart) WriteBinary(w *enc.Writer) { w.WriteString(uint(k.Type()), string(k)) }
func (k hashKeyPart) WriteBinary(w *enc.Writer)   { w.WriteHash(uint(k.Type()), (*[32]byte)(&k)) }
func (k urlKeyPart) WriteBinary(w *enc.Writer)    { w.WriteUrl(uint(k.Type()), k.URL) }
func (k txidKeyPart) WriteBinary(w *enc.Writer)   { w.WriteTxid(uint(k.Type()), k.TxID) }

func (k *intKeyPart) ReadBinary(r *enc.Reader)    { ReadBinary((*int64)(k), r.ReadInt) }
func (k *uintKeyPart) ReadBinary(r *enc.Reader)   { ReadBinary((*uint64)(k), r.ReadUint) }
func (k *stringKeyPart) ReadBinary(r *enc.Reader) { ReadBinary((*string)(k), r.ReadString) }
func (k *hashKeyPart) ReadBinary(r *enc.Reader)   { ReadBinary((*[32]byte)(k), r.ReadHash2) }
func (k *urlKeyPart) ReadBinary(r *enc.Reader)    { ReadBinary(&k.URL, r.ReadUrl) }
func (k *txidKeyPart) ReadBinary(r *enc.Reader)   { ReadBinary(&k.TxID, r.ReadTxid) }

// newKeyPart returns a new key part for the type code.
func newKeyPart(typ typeCode) (keyPart, error) {
	switch typ {
	case typeCodeInt:
		return new(intKeyPart), nil
	case typeCodeUint:
		return new(uintKeyPart), nil
	case typeCodeString:
		return new(stringKeyPart), nil
	case typeCodeHash:
		return new(hashKeyPart), nil
	case typeCodeUrl:
		return new(urlKeyPart), nil
	case typeCodeTxid:
		return new(txidKeyPart), nil
	default:
		return nil, errors.NotAllowed.WithFormat("%v is not a supported key part type", typ)
	}
}

// asKeyPart converts the value to a key part.
func asKeyPart(v any) (keyPart, error) {
	switch v := v.(type) {
	case int64:
		return (*intKeyPart)(&v), nil
	case uint64:
		return (*uintKeyPart)(&v), nil
	case string:
		return (*stringKeyPart)(&v), nil
	case [32]byte:
		return (*hashKeyPart)(&v), nil
	case *url.URL:
		return &urlKeyPart{v}, nil
	case *url.TxID:
		return &txidKeyPart{v}, nil
	default:
		return nil, errors.NotAllowed.WithFormat("%T is not a supported key part type", v)
	}
}

type intKeyPart int64
type uintKeyPart uint64
type stringKeyPart string
type hashKeyPart [32]byte
type urlKeyPart struct{ *url.URL }
type txidKeyPart struct{ *url.TxID }

func (k intKeyPart) Type() typeCode    { return typeCodeInt }
func (k uintKeyPart) Type() typeCode   { return typeCodeUint }
func (k stringKeyPart) Type() typeCode { return typeCodeString }
func (k hashKeyPart) Type() typeCode   { return typeCodeHash }
func (k urlKeyPart) Type() typeCode    { return typeCodeUrl }
func (k txidKeyPart) Type() typeCode   { return typeCodeTxid }

func (k intKeyPart) Value() any    { return int64(k) }
func (k uintKeyPart) Value() any   { return uint64(k) }
func (k stringKeyPart) Value() any { return string(k) }
func (k hashKeyPart) Value() any   { return [32]byte(k) }
func (k urlKeyPart) Value() any    { return k.URL }
func (k txidKeyPart) Value() any   { return k.TxID }

func ReadBinary[V any](v *V, read func(uint) (V, bool)) {
	// Read with field = 0 to tell the reader to skip the field number
	u, _ := read(0)
	*v = u
}

func (k hashKeyPart) MarshalJSON() ([]byte, error) {
	return json.Marshal(hex.EncodeToString(k[:]))
}

func (k *hashKeyPart) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	b, err = hex.DecodeString(s)
	if err != nil {
		return err
	}
	if len(b) != 32 {
		return errors.EncodingError.WithFormat("wrong length for a hash: want 32, got %d", len(b))
	}
	*k = *(*[32]byte)(b)
	return nil
}

func (k *urlKeyPart) UnmarshalJSON(b []byte) error {
	k.URL = new(url.URL)
	return k.URL.UnmarshalJSON(b)
}

func (k *txidKeyPart) UnmarshalJSON(b []byte) error {
	k.TxID = new(url.TxID)
	return k.TxID.UnmarshalJSON(b)
}

// keyPartsEqual returns true if U and V are the same.
func keyPartsEqual(v, u any) bool {
	switch v := v.(type) {
	case int64:
		_, ok := u.(int64)
		if !ok {
			return false
		}
	case uint64:
		_, ok := u.(uint64)
		if !ok {
			return false
		}
	case string:
		_, ok := u.(string)
		if !ok {
			return false
		}
	case [32]byte:
		_, ok := u.([32]byte)
		if !ok {
			return false
		}
	case *url.URL:
		u, ok := u.(*url.URL)
		if !ok {
			return false
		}
		return v.Equal(u)
	case *url.TxID:
		u, ok := u.(*url.TxID)
		if !ok {
			return false
		}
		return v.Equal(u)
	}

	return v == u
}