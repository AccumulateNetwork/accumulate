package encoding

import (
	"encoding"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// ErrInvalidFieldNumber is returned when an invalid field number is encountered.
var ErrInvalidFieldNumber = errors.New("field number is invalid")

// EmptyObject is written when an object would otherwise be empty.
const EmptyObject = 0x80

// A Writer is used to binary-encode a struct and write it to a io.Writer.
//
// If any attempt to write fails, all subsequent write methods are no-ops until
// Err is called.
type Writer struct {
	w       io.Writer
	err     error
	last    uint
	written int
}

// NewWriter creates a new Writer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// didWrite records bytes didWrite and the error if one occurred.
func (w *Writer) didWrite(field uint, written int, err error, format string) {
	w.last = field
	w.written += written

	if err == nil {
		return
	}

	w.err = fmt.Errorf(format+": %w", err)
}

// writeInt writes a varint-encoded signed integer.
func (w *Writer) writeInt(field uint, v int64) {
	if w.err != nil {
		return
	}

	var b [10]byte
	n := binary.PutVarint(b[:], v)
	n, err := w.w.Write(b[:n])
	w.didWrite(field, n, err, "failed to write field")
}

// writeUint writes a varint-encoded unsigned integer.
func (w *Writer) writeUint(field uint, v uint64) {
	if w.err != nil {
		return
	}

	var b [10]byte
	n := binary.PutUvarint(b[:], v)
	n, err := w.w.Write(b[:n])
	w.didWrite(field, n, err, "failed to write field")
}

// writeRaw writes a byte slice.
func (w *Writer) writeRaw(field uint, v []byte) {
	if w.err != nil {
		return
	}

	if len(v) > MaxValueSize {
		w.didWrite(field, 0, fmt.Errorf("too big: %d > %d", len(v), MaxValueSize), "failed to write field")
		return
	}

	n, err := w.w.Write(v)
	w.didWrite(field, n, err, "failed to write field")
}

// didMarshal records the results of marshalling a value.
func (w *Writer) didMarshal(field uint, err error) {
	if w.err != nil {
		return
	}

	if err == nil {
		return
	}

	w.didWrite(field, 0, err, "failed to marshal field")
}

// writeField writes a field number.
func (w *Writer) writeField(field uint) {
	if w.err != nil {
		return
	}

	if field < 1 || field > 32 {
		w.last = field
		w.err = ErrInvalidFieldNumber
		return
	}

	var b [10]byte
	n := binary.PutUvarint(b[:], uint64(field))
	n, err := w.w.Write(b[:n])
	w.didWrite(field, n, err, "failed to write field number")
}

// Reset returns the total number of bytes written, the last field written, and
// an error, if one occurred, and resets the writer.
//
// If a list of field names is provided, the error will be formatted as "field
// <name>: <err>".
func (w *Writer) Reset(fieldNames []string) (written int, lastField uint, err error) {
	if w.written == 0 {
		n, err := w.w.Write([]byte{EmptyObject})
		w.didWrite(0, n, err, "failed to write empty object marker")
	}

	written, lastField, err = w.written, w.last, w.err
	w.written, w.last, w.err = 0, 0, nil

	if fieldNames == nil || err == nil || lastField == 0 {
		return
	}

	if int(lastField) >= len(fieldNames) || fieldNames[lastField] == "" {
		err = fmt.Errorf("field %d: %w", lastField, err)
		return
	}

	err = fmt.Errorf("field %s: %w", fieldNames[lastField], err)
	return
}

// WriteHash writes the value without modification.
func (w *Writer) WriteHash(n uint, v *[32]byte) {
	w.writeField(n)
	w.writeRaw(n, v[:])
}

// WriteInt writes the value as a varint-encoded signed integer.
func (w *Writer) WriteInt(n uint, v int64) {
	w.writeField(n)
	w.writeInt(n, v)
}

// WriteUint writes the value as a varint-encoded unsigned integer.
func (w *Writer) WriteUint(n uint, v uint64) {
	w.writeField(n)
	w.writeUint(n, v)
}

// WriteFloat writes the value as a IEEE 754 encoded floating point number.
func (w *Writer) WriteFloat(n uint, v float64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], math.Float64bits(v))
	w.writeField(n)
	w.writeRaw(n, b[:])
}

// WriteBool writes the value as a varint-encoded unsigned integer.
func (w *Writer) WriteBool(n uint, v bool) {
	var u uint64
	if v {
		u = 1
	}
	w.writeField(n)
	w.writeUint(n, u)
}

// WriteTime writes the value as a varint-encoded UTC Unix timestamp (signed).
func (w *Writer) WriteTime(n uint, v time.Time) {
	w.writeField(n)
	w.writeInt(n, v.UTC().Unix())
}

// WriteBytes writes the length of the value as a varint-encoded unsigned
// integer followed by the value.
func (w *Writer) WriteBytes(n uint, v []byte) {
	w.writeField(n)
	w.writeUint(n, uint64(len(v)))
	w.writeRaw(n, v)
}

// WriteString writes the length of the value as a varint-encoded unsigned
// integer followed by the value.
func (w *Writer) WriteString(n uint, v string) {
	w.writeField(n)
	w.writeUint(n, uint64(len(v)))
	w.writeRaw(n, []byte(v))
}

// WriteDuration writes the value as seconds and nanoseconds, each as a
// varint-encoded unsigned integer.
func (w *Writer) WriteDuration(n uint, v time.Duration) {
	sec, ns := SplitDuration(v)
	w.writeField(n)
	w.writeUint(n, sec)
	w.writeUint(n, ns)
}

// WriteBigInt writes the value as a big-endian byte slice.
func (w *Writer) WriteBigInt(n uint, v *big.Int) {
	if v.Sign() < 0 {
		w.last = n
		w.err = fmt.Errorf("negative big int values are not supported")
		return
	}
	w.WriteBytes(n, v.Bytes())
}

// WriteUrl writes the value as a string.
func (w *Writer) WriteUrl(n uint, v *url.URL) {
	if v == nil {
		w.WriteString(n, "")
	} else {
		w.WriteString(n, v.String())
	}
}

// WriteTxid writes the value as a string.
func (w *Writer) WriteTxid(n uint, v *url.TxID) {
	if v == nil {
		w.WriteString(n, "")
	} else {
		w.WriteString(n, v.String())
	}
}

// WriteValue marshals the value and writes it as a byte slice.
func (w *Writer) WriteValueOld(n uint, v encoding.BinaryMarshaler) {
	b, err := v.MarshalBinary()
	w.didMarshal(n, err)
	w.WriteBytes(n, b)
}

// WriteValue marshals the value and writes it as a byte slice.
func (w *Writer) WriteValue(n uint, marshal func() ([]byte, error)) {
	b, err := marshal()
	w.didMarshal(n, err)
	w.WriteBytes(n, b)
}

// WriteEnum writes the value as a varint-encoded unsigned integer.
func (w *Writer) WriteEnum(n uint, v EnumValueGetter) {
	w.WriteUint(n, v.GetEnumValue())
}
