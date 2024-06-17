// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"
	"io"
	"sort"
)

func closeIfError(err *error, closer io.Closer) {
	if *err != nil {
		_ = closer.Close()
	}
}

func searchIndex(index []byte, start, end int64, hash [32]byte) (int64, bool) {
	i := start + int64(sort.Search(int(end-start), func(i int) bool {
		offset := (start + int64(i)) * indexFileEntrySize
		return bytes.Compare(hash[:], index[offset:offset+32]) <= 0
	}))
	offset := i * indexFileEntrySize
	if i >= end {
		return i, false
	}
	return i, [32]byte(index[offset:offset+32]) == hash
}
