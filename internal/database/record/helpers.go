package record

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

func zero[T any]() (z T) { return z }

func copyValue[T any](v T) T               { return v }
func equalValue[T comparable](a, b T) bool { return a == b }

func copyRef[T interface{ Copy() T }](v T) T             { return v.Copy() }
func equalRef[T interface{ Equal(T) bool }](a, b T) bool { return a.Equal(b) }

func CompareHash(u, v [32]byte) int  { return bytes.Compare(u[:], v[:]) }
func CompareTxid(u, v *url.TxID) int { return u.Compare(v) }
func CompareUrl(u, v *url.URL) int   { return u.Compare(v) }

func ParseString(s string) (string, error) { return s, nil }

type ValueMarshaller[T any] func(value T) ([]byte, error)
type ValueUnmarshaller[T any] func(data []byte) (T, error)

type wrapperFuncs[T any] struct {
	copy      func(T) T
	equal     func(T, T) bool
	marshal   ValueMarshaller[T]
	unmarshal ValueUnmarshaller[T]
}

// UintWrapper defines un/marshalling functions for uint fields.
var UintWrapper = &wrapperFuncs[uint64]{
	copy:      copyValue[uint64],
	equal:     equalValue[uint64],
	marshal:   oldMarshal(encoding.UvarintMarshalBinary),
	unmarshal: encoding.UvarintUnmarshalBinary,
}

// BytesWrapper defines un/marshalling functions for byte slice fields.
var BytesWrapper = &wrapperFuncs[[]byte]{
	copy:      func(v []byte) []byte { u := make([]byte, len(v)); copy(u, v); return u },
	marshal:   oldMarshal(encoding.BytesMarshalBinary),
	unmarshal: encoding.BytesUnmarshalBinary,
}

// HashWrapper defines un/marshalling functions for hash fields.
var HashWrapper = &wrapperFuncs[[32]byte]{
	copy:      copyValue[[32]byte],
	equal:     equalValue[[32]byte],
	marshal:   oldMarshalPtr(encoding.ChainMarshalBinary),
	unmarshal: encoding.ChainUnmarshalBinary,
}

// UrlWrapper defines un/marshalling functions for url fields.
var UrlWrapper = &wrapperFuncs[*url.URL]{
	copy:      copyRef[*url.URL],
	equal:     equalRef[*url.URL],
	marshal:   marshalAsString[*url.URL],
	unmarshal: unmarshalFromString(url.Parse),
}

// TxidWrapper defines un/marshalling functions for txid fields.
var TxidWrapper = &wrapperFuncs[*url.TxID]{
	copy:      copyRef[*url.TxID],
	equal:     equalRef[*url.TxID],
	marshal:   marshalAsString[*url.TxID],
	unmarshal: unmarshalFromString(url.ParseTxID),
}

// StringWrapper defines un/marshalling functions for string fields.
var StringWrapper = &wrapperFuncs[string]{
	copy:      copyValue[string],
	equal:     equalValue[string],
	marshal:   oldMarshal(encoding.StringMarshalBinary),
	unmarshal: encoding.StringUnmarshalBinary,
}

func oldMarshal[T any](fn func(T) []byte) ValueMarshaller[T] {
	return func(value T) ([]byte, error) {
		return fn(value), nil
	}
}

func oldMarshalPtr[T any](fn func(*T) []byte) ValueMarshaller[T] {
	return func(value T) ([]byte, error) {
		return fn(&value), nil
	}
}

func marshalAsString[T fmt.Stringer](v T) ([]byte, error) {
	return encoding.StringMarshalBinary(v.String()), nil
}

func unmarshalFromString[T any](fn func(string) (T, error)) ValueUnmarshaller[T] {
	return func(data []byte) (T, error) {
		s, err := encoding.StringUnmarshalBinary(data)
		if err != nil {
			return zero[T](), errors.Wrap(errors.StatusUnknownError, err)
		}
		v, err := fn(s)
		if err != nil {
			return zero[T](), errors.Wrap(errors.StatusUnknownError, err)
		}
		return v, nil
	}
}
