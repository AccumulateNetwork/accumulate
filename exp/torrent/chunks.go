// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package torrent

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

func ChunksBySize(file io.Reader, size uint64) ([]*ChunkMetadata, [32]byte, error) {
	fileHash := sha256.New()
	buf := make([]byte, size)
	var chunks []*ChunkMetadata
	for i := 0; ; i++ {
		n, err := readAll(file, buf)
		if err != nil {
			return nil, [32]byte{}, err
		}
		if n == 0 {
			break
		}
		fileHash.Write(buf)
		if i%30 == 29 {
			fmt.Println(i)
		}
		chunks = append(chunks, &ChunkMetadata{
			Index:  uint64(i),
			Size:   uint64(n),
			Offset: size * uint64(i),
			Hash:   sha256.Sum256(buf[:n]),
		})
	}

	return chunks, [32]byte(fileHash.Sum(nil)), nil
}

func readAll(file io.Reader, buf []byte) (int, error) {
	var n int
	for n < len(buf) {
		nn, err := file.Read(buf[n:])
		switch {
		case errors.Is(err, io.EOF):
			n += nn
			return n, nil
		case err != nil:
			return n, err
		case nn == 0:
			panic("read nothing")
		default:
			n += nn
		}
	}
	return n, nil
}
