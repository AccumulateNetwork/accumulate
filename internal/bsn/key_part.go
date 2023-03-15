// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"encoding/hex"
	"encoding/json"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	. "gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type keyPart interface {
	Type() TypeCode
	Value() any
	WriteBinary(w *Writer)
	ReadBinary(r *Reader)
}

func newKeyPart(typ TypeCode) (keyPart, error) {
	switch typ {
	case TypeCodeInt:
		return new(intKeyPart), nil
	case TypeCodeUint:
		return new(uintKeyPart), nil
	case TypeCodeString:
		return new(stringKeyPart), nil
	case TypeCodeHash:
		return new(hashKeyPart), nil
	case TypeCodeUrl:
		return new(urlKeyPart), nil
	case TypeCodeTxid:
		return new(txidKeyPart), nil
	default:
		return nil, errors.NotAllowed.WithFormat("%v is not a supported key part type", typ)
	}
}

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

func (k intKeyPart) Type() TypeCode    { return TypeCodeInt }
func (k uintKeyPart) Type() TypeCode   { return TypeCodeUint }
func (k stringKeyPart) Type() TypeCode { return TypeCodeString }
func (k hashKeyPart) Type() TypeCode   { return TypeCodeHash }
func (k urlKeyPart) Type() TypeCode    { return TypeCodeUrl }
func (k txidKeyPart) Type() TypeCode   { return TypeCodeTxid }

func (k intKeyPart) Value() any    { return int64(k) }
func (k uintKeyPart) Value() any   { return uint64(k) }
func (k stringKeyPart) Value() any { return string(k) }
func (k hashKeyPart) Value() any   { return [32]byte(k) }
func (k urlKeyPart) Value() any    { return k.URL }
func (k txidKeyPart) Value() any   { return k.TxID }

func (k intKeyPart) WriteBinary(w *Writer)    { w.WriteInt(uint(k.Type()), int64(k)) }
func (k uintKeyPart) WriteBinary(w *Writer)   { w.WriteUint(uint(k.Type()), uint64(k)) }
func (k stringKeyPart) WriteBinary(w *Writer) { w.WriteString(uint(k.Type()), string(k)) }
func (k hashKeyPart) WriteBinary(w *Writer)   { w.WriteHash(uint(k.Type()), (*[32]byte)(&k)) }
func (k urlKeyPart) WriteBinary(w *Writer)    { w.WriteUrl(uint(k.Type()), k.URL) }
func (k txidKeyPart) WriteBinary(w *Writer)   { w.WriteTxid(uint(k.Type()), k.TxID) }

func (k *intKeyPart) ReadBinary(r *Reader)    { ReadBinary((*int64)(k), r.ReadInt) }
func (k *uintKeyPart) ReadBinary(r *Reader)   { ReadBinary((*uint64)(k), r.ReadUint) }
func (k *stringKeyPart) ReadBinary(r *Reader) { ReadBinary((*string)(k), r.ReadString) }
func (k *hashKeyPart) ReadBinary(r *Reader)   { ReadBinary((*[32]byte)(k), r.ReadHash2) }
func (k *urlKeyPart) ReadBinary(r *Reader)    { ReadBinary(&k.URL, r.ReadUrl) }
func (k *txidKeyPart) ReadBinary(r *Reader)   { ReadBinary(&k.TxID, r.ReadTxid) }

func ReadBinary[V any](v *V, fn func(uint) (V, bool)) {
	u, _ := fn(0)
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
