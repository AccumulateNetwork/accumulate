package encoding

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

var ErrFieldsOutOfOrder = errors.New("fields are out of order")

type bytesReader interface {
	io.Reader
	io.ByteScanner
}

type Reader struct {
	r       bytesReader
	current uint
	seen    []bool
	err     error
	last    uint
}

func NewReader(r io.Reader) *Reader {
	if r, ok := r.(bytesReader); ok {
		return &Reader{r: r}
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

func (r *Reader) readRaw(field uint, n int) ([]byte, bool) {
	if r.err != nil {
		return nil, false
	}

	v := make([]byte, n)
	_, err := io.ReadFull(r.r, v)
	r.didRead(field, err, "failed to read field")
	return v, err == nil
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

	// TODO Does this restore as UTC?
	return time.Unix(v, 0), true
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
	return r.readRaw(n, int(l))
}

// ReadString reads the length of the value as a varint-encoded unsigned integer
// followed by the value.
func (r *Reader) ReadString(n uint) (string, bool) {
	if !r.readField(n) {
		return "", false
	}
	l, _ := r.readUint(n)
	v, ok := r.readRaw(n, int(l))
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
	v, ok := r.readRaw(n, int(l))
	if !ok {
		return nil, false
	}
	return new(big.Int).SetBytes(v), true
}

// ReadUrl reads the value as a string.
func (r *Reader) ReadUrl(n uint) (*url.URL, bool) {
	s, ok := r.ReadString(n)
	if !ok {
		return nil, false
	}
	v, err := url.Parse(s)
	r.didRead(n, err, "failed to parse url")
	return v, err == nil
}

// ReadValue reads the value as a byte slice and unmarshals it.
func (r *Reader) ReadValue(n uint, unmarshal func([]byte) error) bool {
	b, ok := r.ReadBytes(n)
	if !ok {
		return false
	}
	err := unmarshal(b)
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

//ReadAll reads the entire value from the current position
func (r *Reader) ReadAll() ([]byte, error) {
	if r.current != 0 {
		// Return the field number
		err := r.r.UnreadByte()
		if err != nil {
			return nil, err
		}
	}
	return io.ReadAll(r.r)
}
