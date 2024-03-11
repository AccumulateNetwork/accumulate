// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package remote

import (
	"bufio"
	"encoding/binary"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

func read[T any](rd *bufio.Reader, unmarshal func([]byte) (T, error)) (T, error) {
	var z T
	l, err := binary.ReadUvarint(rd)
	if err != nil {
		return z, err
	}

	b := make([]byte, l)
	_, err = io.ReadFull(rd, b)
	if err != nil {
		return z, err
	}

	return unmarshal(b)
}

func write[T encoding.BinaryValue](wr io.Writer, v T) error {
	b, err := v.MarshalBinary()
	if err != nil {
		return errors.EncodingError.Wrap(err)
	}

	var buf [10]byte
	n := binary.PutUvarint(buf[:], uint64(len(b)))
	_, err = wr.Write(buf[:n])
	if err != nil {
		return err
	}

	_, err = wr.Write(b)
	return err
}
