package protocol

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/AccumulateNetwork/accumulated/smt/common"
)

func uvarintBinarySize(v uint64) int {
	return len(uvarintMarshalBinary(v))
}

func uvarintMarshalBinary(v uint64) []byte {
	return common.Uint64Bytes(v)
}

func uvarintUnmarshalBinary(b []byte) (uint64, error) {
	v, n := binary.Uvarint(b)
	if n == 0 {
		return 0, ErrNotEnoughData
	}
	if n < 0 {
		return 0, ErrOverflow
	}
	return v, nil
}

func bytesBinarySize(b []byte) int {
	return len(bytesMarshalBinary(b))
}

func bytesMarshalBinary(b []byte) []byte {
	return common.SliceBytes(b)
}

func bytesUnmarshalBinary(b []byte) ([]byte, error) {
	l, n := binary.Uvarint(b)
	if n == 0 {
		return nil, fmt.Errorf("error decoding length: %w", ErrNotEnoughData)
	}
	if n < 0 {
		return nil, fmt.Errorf("error decoding length: %w", ErrOverflow)
	}
	b = b[n:]
	if len(b) < int(l) {
		return nil, ErrNotEnoughData
	}
	return b[:l], nil
}

func stringBinarySize(s string) int {
	return len(stringMarshalBinary(s))
}

func stringMarshalBinary(s string) []byte {
	return bytesMarshalBinary([]byte(s))
}

func stringUnmarshalBinary(b []byte) (string, error) {
	l, n := binary.Uvarint(b)
	if n == 0 {
		return "", fmt.Errorf("error decoding length: %w", ErrNotEnoughData)
	}
	if n < 0 {
		return "", fmt.Errorf("error decoding length: %w", ErrOverflow)
	}
	b = b[n:]
	if len(b) < int(l) {
		return "", ErrNotEnoughData
	}
	return string(b[:l]), nil
}

func splitDuration(d time.Duration) (sec, ns uint64) {
	sec = uint64(d.Seconds())
	ns = uint64((d - d.Round(time.Second)).Nanoseconds())
	return sec, ns
}

func durationBinarySize(d time.Duration) int {
	sec, ns := splitDuration(d)
	return uvarintBinarySize(sec) + uvarintBinarySize(ns)
}

func durationMarshalBinary(d time.Duration) []byte {
	sec, ns := splitDuration(d)
	return append(uvarintMarshalBinary(sec), uvarintMarshalBinary(ns)...)
}

func durationUnmarshalBinary(b []byte) (time.Duration, error) {
	sec, err := uvarintUnmarshalBinary(b)
	if err != nil {
		return 0, fmt.Errorf("error decoding seconds: %w", err)
	}
	ns, err := uvarintUnmarshalBinary(b)
	if err != nil {
		return 0, fmt.Errorf("error decoding nanoseconds: %w", err)
	}
	return time.Duration(sec)*time.Second + time.Duration(ns), nil
}

func bigintBinarySize(v *big.Int) int {
	return bytesBinarySize(v.Bytes())
}

func bigintMarshalBinary(v *big.Int) []byte {
	// TODO Why aren't we varint encoding this?
	return bytesMarshalBinary(v.Bytes())
}

func bigintUnmarshalBinary(b []byte) (*big.Int, error) {
	b, err := bytesUnmarshalBinary(b)
	if err != nil {
		return nil, fmt.Errorf("error decoding bytes: %w", err)
	}

	v := new(big.Int)
	v.SetBytes(b)
	return v, nil
}

func chainBinarySize(v *[32]byte) int {
	return 32
}

func chainMarshalBinary(v *[32]byte) []byte {
	return (*v)[:]
}

func chainUnmarshalBinary(b []byte) ([32]byte, error) {
	var v [32]byte
	if len(b) < 32 {
		return v, ErrNotEnoughData
	}
	copy(v[:], b)
	return v, nil
}

func chainSetBinarySize(v [][32]byte) int {
	return uvarintBinarySize(uint64(len(v))) + len(v)*32
}

func chainSetMarshalBinary(v [][32]byte) []byte {
	blen := uvarintMarshalBinary(uint64(len(v)))
	b := make([]byte, len(blen)+32*len(v))
	n := copy(b, blen)
	for i := range v {
		n += copy(b[n:], v[i][:])
	}
	return b
}

func chainSetUnmarshalBinary(b []byte) ([][32]byte, error) {
	l, err := uvarintUnmarshalBinary(b)
	if err != nil {
		return nil, fmt.Errorf("error decoding length: %w", err)
	}

	b = b[uvarintBinarySize(l):]
	if len(b) < int(32*l) {
		return nil, ErrNotEnoughData
	}

	v := make([][32]byte, l)
	for i := range v {
		copy(v[i][:], b)
		b = b[32:]
	}
	return v, nil
}

func bytesToJSON(v []byte) string {
	return hex.EncodeToString(v)
}

func chainToJSON(v [32]byte) string {
	return hex.EncodeToString(v[:])
}

func chainSetToJSON(v [][32]byte) []string {
	s := make([]string, len(v))
	for i, v := range v {
		s[i] = chainToJSON(v)
	}
	return s
}

func durationToJSON(v time.Duration) interface{} {
	return v.String()
}

func bytesFromJSON(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

func chainFromJSON(s string) ([32]byte, error) {
	var v [32]byte
	b, err := hex.DecodeString(s)
	if err != nil {
		return v, err
	}
	if copy(v[:], b) < 32 {
		return v, ErrNotEnoughData
	}
	return v, nil
}

func chainSetFromJSON(s []string) ([][32]byte, error) {
	var err error
	v := make([][32]byte, len(s))
	for i, s := range s {
		v[i], err = chainFromJSON(s)
		if err != nil {
			return nil, err
		}
	}
	return v, nil
}

func durationFromJSON(v interface{}) (time.Duration, error) {
	switch v := v.(type) {
	case float64:
		return time.Duration(v * float64(time.Second)), nil
	case int:
		return time.Second * time.Duration(v), nil
	case string:
		return time.ParseDuration(v)
	default:
		return 0, fmt.Errorf("cannot parse %T as a duration", v)
	}
}
