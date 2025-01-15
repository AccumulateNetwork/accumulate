// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"
	stdbin "encoding/binary"
	"errors"
	"io"

	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
)

type lenWriter interface {
	io.Writer
	Len() int
}

type recordWriter struct {
	encBuf *bytes.Buffer
	enc    *binary.Encoder
	lenBuf [4]byte
}

func (w *recordWriter) Write(wr lenWriter, e entry, record []byte) (*recordLocation, error) {
	// Encode the entry
	w.encBuf.Reset()
	w.enc.Reset(w.encBuf)
	err := w.enc.Encode(e)
	if err != nil {
		return nil, err
	}
	if len(record) > 0 {
		_ = w.encBuf.WriteByte(binary.EmptyObject)
	}

	// Encode the length
	loc := new(recordLocation)
	loc.HeaderLen = int64(w.encBuf.Len())
	loc.RecordLen = int64(len(record))
	stdbin.BigEndian.PutUint32(w.lenBuf[:], uint32(loc.HeaderLen+loc.RecordLen))

	// Write the length
	_, e1 := wr.Write(w.lenBuf[:])
	loc.Offset = int64(wr.Len())
	_, e2 := io.Copy(wr, w.encBuf)
	_, e3 := wr.Write(record)
	return loc, errors.Join(e1, e2, e3)
}
