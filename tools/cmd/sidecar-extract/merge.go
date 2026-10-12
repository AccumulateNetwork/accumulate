// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"fmt"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// merge copies every record of src into dst. A key dst already holds with a
// different value is an error, not an overwrite.
func merge(src, dst string) error {
	s, err := leveldb.OpenFile(src, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	if err != nil {
		return err
	}
	defer s.Close()
	d, err := openOutput(dst)
	if err != nil {
		return err
	}
	defer d.Close()

	var added, same int
	it := s.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		v, err := d.Get(it.Key(), nil)
		switch {
		case err == nil && bytes.Equal(v, it.Value()):
			same++
			continue
		case err == nil:
			return fmt.Errorf("%x: %s holds a different value", it.Key(), dst)
		case err != leveldb.ErrNotFound:
			return err
		}
		if err := d.Put(it.Key(), it.Value(), &opt.WriteOptions{Sync: true}); err != nil {
			return err
		}
		added++
	}
	fmt.Printf("merged %s into %s: %d added, %d already there\n", src, dst, added, same)
	return it.Error()
}
