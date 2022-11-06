package message

import (
	"bufio"
	"encoding/binary"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

type Stream = StreamOf[Message]

type StreamOf[T any] interface {
	Read() (T, error)
	Write(T) error
}

type stream[T encoding.BinaryValue] struct {
	raw       io.ReadWriter
	rd        *bufio.Reader
	unmarshal func([]byte) (T, error)
}

// NewStream returns a message stream wrapping the underlying
func NewStream(raw io.ReadWriter) Stream {
	return &stream[Message]{raw, bufio.NewReader(raw), Unmarshal}
}

type valuePtr[T any] interface {
	*T
	encoding.BinaryValue
}

func NewStreamOf[T any, PT valuePtr[T]](raw io.ReadWriter) StreamOf[PT] {
	return &stream[PT]{raw, bufio.NewReader(raw), func(b []byte) (PT, error) {
		v := PT(new(T))
		err := v.UnmarshalBinary(b)
		return v, err
	}}
}

func (s *stream[T]) Read() (T, error) {
	var z T
	l, err := binary.ReadUvarint(s.rd)
	if err != nil {
		return z, err
	}

	b := make([]byte, l)
	_, err = io.ReadFull(s.rd, b)
	if err != nil {
		return z, err
	}

	return s.unmarshal(b)
}

func (s *stream[T]) Write(v T) error {
	b, err := v.MarshalBinary()
	if err != nil {
		return errors.EncodingError.Wrap(err)
	}

	var buf [10]byte
	n := binary.PutUvarint(buf[:], uint64(len(b)))
	_, err = s.raw.Write(buf[:n])
	if err != nil {
		return err
	}

	_, err = s.raw.Write(b)
	return err
}
