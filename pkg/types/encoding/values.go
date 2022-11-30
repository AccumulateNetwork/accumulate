package encoding

import (
	"encoding/binary"
	"fmt"
)

func MarshalUint(v uint64) []byte {
	var buf [16]byte
	n := binary.PutUvarint(buf[:], v)
	return buf[:n]
}

func UnmarshalUint(b []byte) (uint64, error) {
	v, n := binary.Uvarint(b)
	if n == 0 {
		return 0, ErrNotEnoughData
	}
	if n < 0 {
		return 0, ErrOverflow
	}
	return v, nil
}

func MarshalInt(v int64) []byte {
	var buf [16]byte
	n := binary.PutVarint(buf[:], v)
	return buf[:n]
}

func UnmarshalInt(b []byte) (int64, error) {
	v, n := binary.Varint(b)
	if n == 0 {
		return 0, ErrNotEnoughData
	}
	if n < 0 {
		return 0, ErrOverflow
	}
	return v, nil
}

func MarshalBytes(v []byte) []byte {
	b := MarshalUint(uint64(len(v)))
	return append(b, v...)
}

func UnmarshalBytes(b []byte) ([]byte, error) {
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

func MarshalString(s string) []byte {
	return MarshalBytes([]byte(s))
}

func UnmarshalString(b []byte) (string, error) {
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

func MarshalHash(v *[32]byte) []byte {
	return (*v)[:]
}

func UnmarshalHash(b []byte) ([32]byte, error) {
	var v [32]byte
	if len(b) < 32 {
		return v, ErrNotEnoughData
	}
	copy(v[:], b)
	return v, nil
}
