// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"bufio"
	"encoding/binary"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// Stream is a stream of [Message]s.
type Stream = StreamOf[Message]

// Stream is a stream of T.
type StreamOf[T any] interface {
	// Read reads the next value from the stream.
	Read() (T, error)

	// Write writes a value to the stream.
	Write(T) error
}

// stream implements [StreamOf[T]].
type stream[T encoding.BinaryValue] struct {
	raw       io.ReadWriter
	rd        *bufio.Reader
	unmarshal func([]byte) (T, error)
}

// NewStream returns a [Stream] of [Message]s wrapping the underlying
// [io.ReadWriter].
//
// Messages are marshalled and unmarshalled as binary prefixed with the byte
// count.
func NewStream(raw io.ReadWriter) Stream {
	return &stream[Message]{raw, bufio.NewReader(raw), Unmarshal}
}

type valuePtr[T any] interface {
	*T
	encoding.BinaryValue
}

// NewStreamOf returns a [StreamOf[T]] wrapping the underlying [io.ReadWriter].
// A pointer to T must implement [encoding.BinaryValue].
//
// Values are marshalled and unmarshalled as binary prefixed with the byte
// count.
func NewStreamOf[T any, PT valuePtr[T]](raw io.ReadWriter) StreamOf[PT] {
	return &stream[PT]{raw, bufio.NewReader(raw), func(b []byte) (PT, error) {
		v := PT(new(T))
		err := v.UnmarshalBinary(b)
		return v, err
	}}
}

// Read reads and unmarshals the next value.
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

// Write writes a value.
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
