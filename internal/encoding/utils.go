// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/smt/common"
)

// Some of these methods have no parameters because they are used by generated
// code

func BytesCopy(v []byte) []byte {
	u := make([]byte, len(v))
	copy(u, v)
	return v
}

func BigintCopy(v *big.Int) *big.Int {
	u := new(big.Int)
	u.Set(v)
	return u
}

func BoolBinarySize(_ bool) int {
	return 1
}

func BoolMarshalBinary(v bool) []byte {
	if v {
		return []byte{1}
	}
	return []byte{0}
}

func BoolUnmarshalBinary(b []byte) (bool, error) {
	if len(b) == 0 {
		return false, ErrNotEnoughData
	}
	switch b[0] {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("%d is not a valid boolean", b[0])
	}
}

func TimeBinarySize(v time.Time) int {
	return VarintBinarySize(v.UTC().Unix())
}

func TimeMarshalBinary(v time.Time) []byte {
	return VarintMarshalBinary(v.UTC().Unix())
}

func TimeUnmarshalBinary(b []byte) (time.Time, error) {
	v, err := VarintUnmarshalBinary(b)
	if err != nil {
		return time.Time{}, err
	}

	// TODO Does this restore as UTC?
	return time.Unix(v, 0), nil
}

func UvarintBinarySize(v uint64) int {
	return len(UvarintMarshalBinary(v))
}

func UvarintMarshalBinary(v uint64) []byte {
	return common.Uint64Bytes(v)
}

func UvarintUnmarshalBinary(b []byte) (uint64, error) {
	v, n := binary.Uvarint(b)
	if n == 0 {
		return 0, ErrNotEnoughData
	}
	if n < 0 {
		return 0, ErrOverflow
	}
	return v, nil
}

func VarintBinarySize(v int64) int {
	return len(VarintMarshalBinary(v))
}

func VarintMarshalBinary(v int64) []byte {
	return common.Int64Bytes(v)
}

func VarintUnmarshalBinary(b []byte) (int64, error) {
	v, n := binary.Varint(b)
	if n == 0 {
		return 0, ErrNotEnoughData
	}
	if n < 0 {
		return 0, ErrOverflow
	}
	return v, nil
}

func BytesBinarySize(b []byte) int {
	return len(BytesMarshalBinary(b))
}

func BytesMarshalBinary(b []byte) []byte {
	return common.SliceBytes(b)
}

func BytesUnmarshalBinary(b []byte) ([]byte, error) {
	l, n := binary.Uvarint(b)
	if n == 0 {
		return nil, fmt.Errorf("error decoding length: %w", ErrNotEnoughData)
	}
	if n < 0 {
		return nil, fmt.Errorf("error decoding length: %w", ErrOverflow)
	}
	if l == 0 {
		return nil, nil
	}
	b = b[n:]
	if len(b) < int(l) {
		return nil, ErrNotEnoughData
	}
	return b[:l], nil
}

func StringBinarySize(s string) int {
	return len(StringMarshalBinary(s))
}

func StringMarshalBinary(s string) []byte {
	return BytesMarshalBinary([]byte(s))
}

func StringUnmarshalBinary(b []byte) (string, error) {
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

func DurationBinarySize(d time.Duration) int {
	sec, ns := SplitDuration(d)
	return UvarintBinarySize(sec) + UvarintBinarySize(ns)
}

func DurationMarshalBinary(d time.Duration) []byte {
	sec, ns := SplitDuration(d)
	return append(UvarintMarshalBinary(sec), UvarintMarshalBinary(ns)...)
}

func DurationUnmarshalBinary(b []byte) (time.Duration, error) {
	sec, err := UvarintUnmarshalBinary(b)
	if err != nil {
		return 0, fmt.Errorf("error decoding seconds: %w", err)
	}
	ns, err := UvarintUnmarshalBinary(b)
	if err != nil {
		return 0, fmt.Errorf("error decoding nanoseconds: %w", err)
	}
	return time.Duration(sec)*time.Second + time.Duration(ns), nil
}

func BigintBinarySize(v *big.Int) int {
	return BytesBinarySize(v.Bytes())
}

func BigintMarshalBinary(v *big.Int) []byte {
	// TODO Why aren't we varint encoding this?
	return BytesMarshalBinary(v.Bytes())
}

func BigintUnmarshalBinary(b []byte) (*big.Int, error) {
	b, err := BytesUnmarshalBinary(b)
	if err != nil {
		return nil, fmt.Errorf("error decoding bytes: %w", err)
	}

	v := new(big.Int)
	v.SetBytes(b)
	return v, nil
}

// ToDo:  Why a parameter? It does nothing...
func ChainBinarySize(v *[32]byte) int {
	return 32
}

func ChainMarshalBinary(v *[32]byte) []byte {
	return (*v)[:]
}

func ChainUnmarshalBinary(b []byte) ([32]byte, error) {
	var v [32]byte
	if len(b) < 32 {
		return v, ErrNotEnoughData
	}
	copy(v[:], b)
	return v, nil
}

func ChainSetBinarySize(v [][32]byte) int {
	return UvarintBinarySize(uint64(len(v))) + len(v)*32
}

func ChainSetMarshalBinary(v [][32]byte) []byte {
	blen := UvarintMarshalBinary(uint64(len(v)))
	b := make([]byte, len(blen)+32*len(v))
	n := copy(b, blen)
	for i := range v {
		n += copy(b[n:], v[i][:])
	}
	return b
}

func ChainSetUnmarshalBinary(b []byte) ([][32]byte, error) {
	l, err := UvarintUnmarshalBinary(b)
	if err != nil {
		return nil, fmt.Errorf("error decoding length: %w", err)
	}

	b = b[UvarintBinarySize(l):]
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

func UnmarshalEnumType(r io.Reader, value EnumValueSetter) error {
	reader := NewReader(r)
	v, ok := reader.ReadUint(1)
	_, err := reader.Reset([]string{"Type"})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("field Type: missing")
	}

	if !value.SetEnumValue(v) {
		return fmt.Errorf("field Type: invalid value %d", v)
	}
	return nil
}

// SetPtr sets *target = value
func SetPtr(value, target interface{}) (err error) {
	if value == nil {
		panic("value is nil")
	}
	if target == nil {
		panic("target is nil")
	}

	// Make sure target is a non-nil pointer
	rtarget := reflect.ValueOf(target)
	if rtarget.Kind() != reflect.Ptr {
		panic(fmt.Errorf("target %T is not a pointer", target))
	}
	if rtarget.IsNil() {
		panic("target is nil")
	}
	rtarget = rtarget.Elem()

	// If target is a pointer to value, there's nothing to do
	rvalue := reflect.ValueOf(value)
	if rvalue.Kind() == reflect.Ptr && rtarget.Kind() == reflect.Ptr && rvalue.Pointer() == rtarget.Pointer() {
		return nil
	}

	// Check if we can: *target = value
	if !rvalue.Type().AssignableTo(rtarget.Type()) {
		return fmt.Errorf("cannot assign %T to %v", value, rtarget.Type())
	}

	rtarget.Set(rvalue)
	return nil
}
