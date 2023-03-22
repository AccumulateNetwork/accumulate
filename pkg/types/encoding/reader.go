// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

var ErrFieldsOutOfOrder = errors.New("fields are out of order")

const MaxValueSize = 1 << 15

type BytesReader interface {
	io.Reader
	io.ByteScanner
}

type Reader struct {
	r       BytesReader
	current uint
	seen    []bool
	err     error
	last    uint

	IgnoreSizeLimit bool
}

func NewReader(r io.Reader) *Reader {
	if br, ok := r.(BytesReader); ok {
		return &Reader{r: br}
	} else {
		return &Reader{r: bufio.NewReader(r)}
	}
}

func (r *Reader) didRead(field uint, err error, format string) {
	if r.err != nil {
		return
	}

	r.last = field

	if err == nil {
		return
	}

	r.err = fmt.Errorf(format+": %w", err)
}

func (r *Reader) readInt(field uint) (int64, bool) {
	if r.err != nil {
		return 0, false
	}

	v, err := binary.ReadVarint(r.r)
	r.didRead(field, err, "failed to read field")
	return v, err == nil
}

func (r *Reader) readUint(field uint) (uint64, bool) {
	if r.err != nil {
		return 0, false
	}

	v, err := binary.ReadUvarint(r.r)
	r.didRead(field, err, "failed to read field")
	return v, err == nil
}

func (r *Reader) readRaw(field uint, n uint64) ([]byte, bool) {
	if r.err != nil {
		return nil, false
	}

	if n == 0 {
		return nil, true
	}

	if rl, ok := r.r.(interface{ Len() int }); ok && uint64(rl.Len()) < n {
		r.didRead(field, io.EOF, "failed to read field")
	}

	if !r.IgnoreSizeLimit && n >= MaxValueSize {
		r.didRead(field, fmt.Errorf("too big: %d > %d", n, MaxValueSize), "failed to read field")
		return nil, false
	}

	v := make([]byte, n)
	_, err := io.ReadFull(r.r, v)
	r.didRead(field, err, "failed to read field")
	return v, err == nil
}

func (r *Reader) readLimited(field uint, n uint64) (io.Reader, bool) {
	if r.err != nil {
		return nil, false
	}

	if n == 0 {
		return bytes.NewReader([]byte{}), true
	}

	if rl, ok := r.r.(interface{ Len() int }); ok && uint64(rl.Len()) < n {
		r.didRead(field, io.EOF, "failed to read field")
	}

	return io.LimitReader(r.r, int64(n)), true
}

// Reset returns a list of seen fields and an error, if one occurred, and resets
// the reader.
//
// If a list of field names is provided, the error will be formatted as "field
// <name>: <err>".
func (r *Reader) Reset(fieldNames []string) (seen []bool, err error) {
	seen, last, err := r.seen, r.last, r.err
	r.seen, r.last, r.err = nil, 0, nil

	if fieldNames == nil || err == nil || last == 0 {
		return
	}

	if int(last) >= len(fieldNames) || fieldNames[last] == "" {
		err = fmt.Errorf("field %d: %w", last, err)
		return
	}

	err = fmt.Errorf("field %s: %w", fieldNames[last], err)
	return
}

func (r *Reader) nextField() bool {
	if len(r.seen) == 0 {
		b, err := r.r.ReadByte()
		if errors.Is(err, io.EOF) {
			return false
		}
		if err != nil {
			r.last = 0
			r.err = fmt.Errorf("failed to read field number: %w", err)
			return false
		}
		if b == EmptyObject {
			r.current = EmptyObject
			return false
		}
		err = r.r.UnreadByte()
		if err != nil {
			r.last = 0
			r.err = fmt.Errorf("failed to read field number: %w", err)
			return false
		}
	}

	v, err := binary.ReadUvarint(r.r)
	if errors.Is(err, io.EOF) {
		return false
	}
	if err != nil {
		r.last = 0
		r.err = fmt.Errorf("failed to read field number: %w", err)
		return false
	}
	if v < 1 || v > 32 {
		r.last = 0
		r.err = fmt.Errorf("failed to read field number: %w", ErrInvalidFieldNumber)
		return false
	}
	r.current = uint(v)

	for len(r.seen) < int(v-1) {
		r.seen = append(r.seen, false)
	}
	r.seen = append(r.seen, true)
	return true
}

func (r *Reader) readField(field uint) bool {
	if r.err != nil {
		return false
	}
	if field == 0 {
		return r.current == 0 && r.last == 0
	}

	if field < 1 || field > 32 {
		r.err = ErrInvalidFieldNumber
		return false
	}

	if r.current == 0 {
		if !r.nextField() {
			return false
		}
	}

	if r.current < field {
		r.last = 0
		r.err = ErrFieldsOutOfOrder
		return false
	}

	if r.current > field {
		return false
	}

	r.current = 0
	return true
}

// ReadHash reads 32 bytes.
func (r *Reader) ReadHash(n uint) (*[32]byte, bool) {
	if !r.readField(n) {
		return nil, false
	}
	v, ok := r.readRaw(n, 32)
	if !ok {
		return nil, false
	}
	return (*[32]byte)(v), true
}

func (r *Reader) ReadHash2(n uint) ([32]byte, bool) {
	v, ok := r.ReadHash(n)
	if !ok {
		return [32]byte{}, false
	}
	return *v, true
}

// ReadInt reads the value as a varint-encoded signed integer.
func (r *Reader) ReadInt(n uint) (int64, bool) {
	if !r.readField(n) {
		return 0, false
	}
	return r.readInt(n)
}

// ReadUint reads the value as a varint-encoded unsigned integer.
func (r *Reader) ReadUint(n uint) (uint64, bool) {
	if !r.readField(n) {
		return 0, false
	}
	return r.readUint(n)
}

// ReadFloat reads the value as a varint-encoded unsigned integer.
func (r *Reader) ReadFloat(n uint) (float64, bool) {
	if !r.readField(n) {
		return 0, false
	}
	b, ok := r.readRaw(n, 8)
	if !ok {
		return 0, false
	}
	v := math.Float64frombits(binary.BigEndian.Uint64(b))
	return v, true
}

// ReadBool reads the value as a varint-encoded unsigned integer. An error is
// recorded if the value is not 0 or 1.
func (r *Reader) ReadBool(n uint) (bool, bool) {
	if !r.readField(n) {
		return false, false
	}
	v, ok := r.readUint(n)
	if !ok {
		return false, false
	}
	switch v {
	case 0:
		return false, true
	case 1:
		return true, true
	default:
		r.didRead(n, fmt.Errorf("%d is not a valid boolean", v), "failed to unmarshal field")
		return false, false
	}
}

// ReadTime reads the value as a varint-encoded UTC Unix timestamp (signed).
func (r *Reader) ReadTime(n uint) (time.Time, bool) {
	if !r.readField(n) {
		return time.Time{}, false
	}
	v, ok := r.readInt(n)
	if !ok {
		return time.Time{}, false
	}

	return time.Unix(v, 0).UTC(), true
}

// ReadBytes reads the length of the value as a varint-encoded unsigned integer
// followed by the value.
func (r *Reader) ReadBytes(n uint) ([]byte, bool) {
	if !r.readField(n) {
		return nil, false
	}
	l, ok := r.readUint(n)
	if !ok {
		return nil, false
	}
	return r.readRaw(n, l)
}

// ReadString reads the length of the value as a varint-encoded unsigned integer
// followed by the value.
func (r *Reader) ReadString(n uint) (string, bool) {
	if !r.readField(n) {
		return "", false
	}
	l, _ := r.readUint(n)
	v, ok := r.readRaw(n, l)
	if !ok {
		return "", false
	}
	return string(v), true
}

// ReadDuration reads the value as seconds and nanoseconds, each as a
// varint-encoded unsigned integer.
func (r *Reader) ReadDuration(n uint) (time.Duration, bool) {
	if !r.readField(n) {
		return 0, false
	}
	sec, _ := r.readUint(n)
	ns, ok := r.readUint(n)
	if !ok {
		return 0, false
	}
	return time.Duration(sec)*time.Second + time.Duration(ns), true
}

// ReadBigInt reads the value as a big-endian byte slice.
func (r *Reader) ReadBigInt(n uint) (*big.Int, bool) {
	if !r.readField(n) {
		return nil, false
	}
	l, _ := r.readUint(n)
	v, ok := r.readRaw(n, l)
	if !ok {
		return nil, false
	}
	return new(big.Int).SetBytes(v), true
}

// ReadUrl reads the value as a URL.
func (r *Reader) ReadUrl(n uint) (*url.URL, bool) {
	s, ok := r.ReadString(n)
	if !ok {
		return nil, false
	}
	if s == "" {
		return nil, true
	}
	v, err := url.Parse(s)
	r.didRead(n, err, "failed to parse url")
	return v, err == nil
}

// ReadTxid reads the value as a transaction ID.
func (r *Reader) ReadTxid(n uint) (*url.TxID, bool) {
	s, ok := r.ReadString(n)
	if !ok {
		return nil, false
	}
	if s == "" {
		return nil, true
	}
	v, err := url.ParseTxID(s)
	r.didRead(n, err, "failed to parse url")
	return v, err == nil
}

// ReadValue reads the value as a byte slice and unmarshals it.
func (r *Reader) ReadValue(n uint, unmarshal func(io.Reader) error) bool {
	if !r.readField(n) {
		return false
	}
	l, ok := r.readUint(n)
	if !ok {
		return false
	}

	rd, ok := r.readLimited(n, l)
	if !ok {
		return false
	}
	err := unmarshal(rd)
	r.didRead(n, err, "failed to unmarshal value")
	return err == nil
}

func (r *Reader) ReadEnum(n uint, v EnumValueSetter) bool {
	u, ok := r.ReadUint(n)
	if !ok {
		return false
	}
	if v.SetEnumValue(u) {
		return true
	}
	r.didRead(n, fmt.Errorf("%d is not a valid value", u), "failed to unmarshal value")
	return false
}

// IsEmpty returns true if the object is empty. IsEmpty will always return false
// if called prior to any other Read method.
func (r *Reader) IsEmpty() bool { return r.current == EmptyObject }

// ReadAll reads the entire value from the current position
func (r *Reader) ReadAll() ([]byte, error) {
	if r.current == EmptyObject {
		return nil, nil
	}
	if r.current != 0 {
		// Return the field number
		err := r.r.UnreadByte()
		if err != nil {
			return nil, err
		}
	}
	return io.ReadAll(r.r)
}
