package utils

import (
	"encoding/binary"
	"errors"
)

var ErrNotEnoughData = errors.New("not enough data")
var ErrOverflow = errors.New("overflow")

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
