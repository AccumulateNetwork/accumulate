// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package record

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	enc "gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	binary2 "gitlab.com/accumulatenetwork/core/schema/pkg/binary"
)

// keyPartRd is a part of a [Key]. keyPartRd is used for marshalling.
type keyPartRd interface {
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
	WriteBinary(*enc.Writer)

	WriteBinary2(*binary2.Encoder) error
}

type keyPartWr interface {
	keyPartRd

	// ReadBinary reads the key part from the binary reader.
	//
	// The intended use of the binary reader is reading field-number tagged
	// struct field values. The binary reader _does_ pay attention to field
	// numbers, and regardless we don't know the 'field numbers' ahead of time
	// (since they are actually type codes) so we have to circumvent the field
	// number logic. [Key.UnmarshalBinaryFrom] handles reading the type codes,
	// so here we tell the binary reader we want to read field zero, which
	// instructs it to skip the field number logic and read the value directly.
	ReadBinary(*enc.Reader)

	ReadBinary2(*binary2.Decoder) error
}

type enc2 = binary2.Encoder
type dec2 = binary2.Decoder

// These methods are here to keep them close to the explanation above.

func (k intKeyPart) WriteBinary(w *enc.Writer)    { w.WriteInt(uint(k.Type()), int64(k)) }
func (k uintKeyPart) WriteBinary(w *enc.Writer)   { w.WriteUint(uint(k.Type()), uint64(k)) }
func (k stringKeyPart) WriteBinary(w *enc.Writer) { w.WriteString(uint(k.Type()), string(k)) }
func (k hashKeyPart) WriteBinary(w *enc.Writer)   { w.WriteHash(uint(k.Type()), (*[32]byte)(&k)) }
func (k bytesKeyPart) WriteBinary(w *enc.Writer)  { w.WriteBytes(uint(k.Type()), k) }
func (k *urlKeyPart) WriteBinary(w *enc.Writer)   { w.WriteUrl(uint(k.Type()), (*url.URL)(k)) }
func (k *txidKeyPart) WriteBinary(w *enc.Writer)  { w.WriteTxid(uint(k.Type()), (*url.TxID)(k)) }
func (k timeKeyPart) WriteBinary(w *enc.Writer)   { w.WriteTime(uint(k.Type()), time.Time(k)) }

func (k *intKeyPart) ReadBinary(r *enc.Reader)    { rdBinVal((*int64)(k), r.ReadInt) }
func (k *uintKeyPart) ReadBinary(r *enc.Reader)   { rdBinVal((*uint64)(k), r.ReadUint) }
func (k *stringKeyPart) ReadBinary(r *enc.Reader) { rdBinVal((*string)(k), r.ReadString) }
func (k *hashKeyPart) ReadBinary(r *enc.Reader)   { rdBinVal((*[32]byte)(k), r.ReadHash2) }
func (k *bytesKeyPart) ReadBinary(r *enc.Reader)  { rdBinVal((*[]byte)(k), r.ReadBytes) }
func (k *urlKeyPart) ReadBinary(r *enc.Reader)    { rdBinPtr((*url.URL)(k), r.ReadUrl) }
func (k *txidKeyPart) ReadBinary(r *enc.Reader)   { rdBinPtr((*url.TxID)(k), r.ReadTxid) }
func (k *timeKeyPart) ReadBinary(r *enc.Reader)   { rdBinVal((*time.Time)(k), r.ReadTime) }

func (k intKeyPart) WriteBinary2(w *enc2) error    { return w.EncodeInt(int64(k)) }
func (k uintKeyPart) WriteBinary2(w *enc2) error   { return w.EncodeUint(uint64(k)) }
func (k stringKeyPart) WriteBinary2(w *enc2) error { return w.EncodeString(string(k)) }
func (k hashKeyPart) WriteBinary2(w *enc2) error   { return w.EncodeHash(k) }
func (k bytesKeyPart) WriteBinary2(w *enc2) error  { return w.EncodeBytes(k) }
func (k *urlKeyPart) WriteBinary2(w *enc2) error   { return w.EncodeString((*url.URL)(k).String()) }
func (k *txidKeyPart) WriteBinary2(w *enc2) error  { return w.EncodeString((*url.TxID)(k).String()) }
func (k timeKeyPart) WriteBinary2(w *enc2) error   { return w.EncodeInt(time.Time(k).UTC().Unix()) }

func (k *intKeyPart) ReadBinary2(r *dec2) error    { return rdBin2((*int64)(k), r.DecodeInt) }
func (k *uintKeyPart) ReadBinary2(r *dec2) error   { return rdBin2((*uint64)(k), r.DecodeUint) }
func (k *stringKeyPart) ReadBinary2(r *dec2) error { return rdBin2((*string)(k), r.DecodeString) }
func (k *hashKeyPart) ReadBinary2(r *dec2) error   { return rdBin2((*[32]byte)(k), r.DecodeHash) }
func (k *bytesKeyPart) ReadBinary2(r *dec2) error  { return rdBin2((*[]byte)(k), r.DecodeBytes) }
func (k *urlKeyPart) ReadBinary2(r *dec2) error    { return r.DecodeValueV2((*url.URL)(k)) }
func (k *txidKeyPart) ReadBinary2(r *dec2) error   { return r.DecodeValueV2((*url.TxID)(k)) }

func (k *timeKeyPart) ReadBinary2(r *dec2) error {
	v, err := r.DecodeInt()
	*k = timeKeyPart(time.Unix(v, 0).UTC())
	return err
}

// newKeyPart returns a new key part for the type code.
func newKeyPart(typ typeCode) (keyPartWr, error) {
	switch typ {
	case typeCodeInt:
		return new(intKeyPart), nil
	case typeCodeUint:
		return new(uintKeyPart), nil
	case typeCodeString:
		return new(stringKeyPart), nil
	case typeCodeHash:
		return new(hashKeyPart), nil
	case typeCodeBytes:
		return new(bytesKeyPart), nil
	case typeCodeUrl:
		return new(urlKeyPart), nil
	case typeCodeTxid:
		return new(txidKeyPart), nil
	case typeCodeTime:
		return new(timeKeyPart), nil
	default:
		return nil, errors.NotAllowed.WithFormat("%v is not a supported key part type", typ)
	}
}

// asKeyPart converts the value to a key part.
func asKeyPart(v any) (keyPartRd, error) {
	switch v := v.(type) {
	case int64:
		return intKeyPart(v), nil
	case int:
		u := int64(v)
		return intKeyPart(u), nil
	case uint64:
		return uintKeyPart(v), nil
	case uint:
		u := uint64(v)
		return uintKeyPart(u), nil
	case string:
		return stringKeyPart(v), nil
	case [32]byte:
		return hashKeyPart(v), nil
	case []byte:
		return bytesKeyPart(v), nil
	case *url.URL:
		return (*urlKeyPart)(v), nil
	case *url.TxID:
		return (*txidKeyPart)(v), nil
	case time.Time:
		return timeKeyPart(v), nil
	default:
		return nil, errors.NotAllowed.WithFormat("%T is not a supported key part type", v)
	}
}

func keyPartTypeOf(v any) (typeCode, bool) {
	switch v.(type) {
	case int64, int:
		return typeCodeInt, true
	case uint64, uint:
		return typeCodeUint, true
	case string:
		return typeCodeString, true
	case [32]byte:
		return typeCodeHash, true
	case []byte:
		return typeCodeBytes, true
	case *url.URL:
		return typeCodeUrl, true
	case *url.TxID:
		return typeCodeTxid, true
	case time.Time:
		return typeCodeTime, true
	default:
		return 0, false
	}
}

type intKeyPart int64
type uintKeyPart uint64
type stringKeyPart string
type hashKeyPart [32]byte
type bytesKeyPart []byte
type urlKeyPart url.URL
type txidKeyPart url.TxID
type timeKeyPart time.Time

func (k intKeyPart) Type() typeCode    { return typeCodeInt }
func (k uintKeyPart) Type() typeCode   { return typeCodeUint }
func (k stringKeyPart) Type() typeCode { return typeCodeString }
func (k hashKeyPart) Type() typeCode   { return typeCodeHash }
func (k bytesKeyPart) Type() typeCode  { return typeCodeBytes }
func (k urlKeyPart) Type() typeCode    { return typeCodeUrl }
func (k txidKeyPart) Type() typeCode   { return typeCodeTxid }
func (k timeKeyPart) Type() typeCode   { return typeCodeTime }

func (k intKeyPart) Value() any    { return int64(k) }
func (k uintKeyPart) Value() any   { return uint64(k) }
func (k stringKeyPart) Value() any { return string(k) }
func (k hashKeyPart) Value() any   { return [32]byte(k) }
func (k bytesKeyPart) Value() any  { return []byte(k) }
func (k *urlKeyPart) Value() any   { return (*url.URL)(k) }
func (k *txidKeyPart) Value() any  { return (*url.TxID)(k) }
func (k timeKeyPart) Value() any   { return time.Time(k) }

func rdBinVal[V any](v *V, read func(uint) (V, bool)) {
	// Read with field = 0 to tell the reader to skip the field number
	u, _ := read(0)
	*v = u
}

func rdBinPtr[V any](v *V, read func(uint) (*V, bool)) {
	// Read with field = 0 to tell the reader to skip the field number
	u, _ := read(0)
	*v = *u
}

func rdBin2[V any](v *V, read func() (V, error)) error {
	u, err := read()
	*v = u
	return err
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

func (k bytesKeyPart) MarshalJSON() ([]byte, error) {
	return json.Marshal(hex.EncodeToString(k[:]))
}

func (k *bytesKeyPart) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	*k, err = hex.DecodeString(s)
	if err != nil {
		return err
	}
	return nil
}

func (k *urlKeyPart) UnmarshalJSON(b []byte) error {
	return (*url.URL)(k).UnmarshalJSON(b)
}

func (k *txidKeyPart) UnmarshalJSON(b []byte) error {
	return (*url.TxID)(k).UnmarshalJSON(b)
}

func (k *timeKeyPart) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, (*time.Time)(k))
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
	case []byte:
		u, ok := u.([]byte)
		if !ok {
			return false
		}
		return bytes.Equal(v, u)
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
	case time.Time:
		u, ok := u.(time.Time)
		if !ok {
			return false
		}
		return v.Equal(u)
	}

	return v == u
}

func keyPartsCompare(v, u any) int {
	a, ok := keyPartTypeOf(v)
	if !ok {
		panic(errors.NotAllowed.WithFormat("%T is not a supported key part type", v))
	}
	b, ok := keyPartTypeOf(u)
	if !ok {
		panic(errors.NotAllowed.WithFormat("%T is not a supported key part type", u))
	}
	if a != b {
		return int(a) - int(b)
	}

	switch a {
	case typeCodeInt:
		var i, j int64
		if x, ok := v.(int); ok {
			i = int64(x)
		} else {
			i = v.(int64)
		}
		if x, ok := u.(int); ok {
			j = int64(x)
		} else {
			j = u.(int64)
		}
		if i < j {
			return -1
		} else if i > j {
			return +1
		}
		return 0
	case typeCodeUint:
		var i, j uint64
		if x, ok := v.(uint); ok {
			i = uint64(x)
		} else {
			i = v.(uint64)
		}
		if x, ok := u.(uint); ok {
			j = uint64(x)
		} else {
			j = u.(uint64)
		}
		if i < j {
			return -1
		} else if i > j {
			return +1
		}
		return 0
	case typeCodeString:
		v, u := v.(string), u.(string)
		return strings.Compare(v, u)
	case typeCodeHash:
		v, u := v.([32]byte), u.([32]byte)
		return bytes.Compare(v[:], u[:])
	case typeCodeBytes:
		v, u := v.([]byte), u.([]byte)
		return bytes.Compare(v, u)
	case typeCodeUrl:
		v, u := v.(*url.URL), u.(*url.URL)
		return (*url.URL)(v).Compare(u)
	case typeCodeTxid:
		v, u := v.(*url.TxID), u.(*url.TxID)
		return v.Compare(u)
	case typeCodeTime:
		v, u := v.(time.Time), u.(time.Time)
		return v.Compare(u)
	default:
		panic("unknown type")
	}
}
