package database

import (
	"bytes"
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

func compareHash(u, v [32]byte) int  { return bytes.Compare(u[:], v[:]) }
func compareTxid(u, v *url.TxID) int { return u.Compare(v) }
func compareUrl(u, v *url.URL) int   { return u.Compare(v) }

func parseString(s string) (string, error) { return s, nil }

func (e *SignatureEntry) Compare(f *SignatureEntry) int {
	return bytes.Compare(e.SignatureHash[:], f.SignatureHash[:])
}

func (e *TransactionChainEntry) Compare(f *TransactionChainEntry) int {
	v := e.Account.Compare(f.Account)
	if v != 0 {
		return v
	}
	return strings.Compare(e.Chain, f.Chain)
}

type valueMarshaller[T any] func(value T) ([]byte, error)
type valueUnmarshaller[T any] func(data []byte) (T, error)

type wrapperFuncs[T any] struct {
	copy      func(T) T
	marshal   valueMarshaller[T]
	unmarshal valueUnmarshaller[T]
}

var uintWrapper = &wrapperFuncs[uint64]{
	copy:      copyValue[uint64],
	marshal:   oldMarshal(encoding.UvarintMarshalBinary),
	unmarshal: encoding.UvarintUnmarshalBinary,
}

var bytesWrapper = &wrapperFuncs[[]byte]{
	copy:      func(v []byte) []byte { u := make([]byte, len(v)); copy(u, v); return u },
	marshal:   oldMarshal(encoding.BytesMarshalBinary),
	unmarshal: encoding.BytesUnmarshalBinary,
}

var hashWrapper = &wrapperFuncs[[32]byte]{
	copy:      copyValue[[32]byte],
	marshal:   oldMarshalPtr(encoding.ChainMarshalBinary),
	unmarshal: encoding.ChainUnmarshalBinary,
}

var urlWrapper = &wrapperFuncs[*url.URL]{
	copy:      copyRef[*url.URL],
	marshal:   marshalAsString[*url.URL],
	unmarshal: unmarshalFromString(url.Parse),
}

var txidWrapper = &wrapperFuncs[*url.TxID]{
	copy:      copyRef[*url.TxID],
	marshal:   marshalAsString[*url.TxID],
	unmarshal: unmarshalFromString(url.ParseTxID),
}

func copyValue[T any](v T) T { return v }

func copyRef[T interface{ Copy() T }](v T) T { return v.Copy() }

func oldMarshal[T any](fn func(T) []byte) valueMarshaller[T] {
	return func(value T) ([]byte, error) {
		return fn(value), nil
	}
}

func oldMarshalPtr[T any](fn func(*T) []byte) valueMarshaller[T] {
	return func(value T) ([]byte, error) {
		return fn(&value), nil
	}
}

func marshalAsString[T fmt.Stringer](v T) ([]byte, error) {
	return encoding.StringMarshalBinary(v.String()), nil
}

func unmarshalFromString[T any](fn func(string) (T, error)) valueUnmarshaller[T] {
	return func(data []byte) (T, error) {
		s, err := encoding.StringUnmarshalBinary(data)
		if err != nil {
			return zero[T](), errors.Wrap(errors.StatusUnknown, err)
		}
		v, err := fn(s)
		if err != nil {
			return zero[T](), errors.Wrap(errors.StatusUnknown, err)
		}
		return v, nil
	}
}
