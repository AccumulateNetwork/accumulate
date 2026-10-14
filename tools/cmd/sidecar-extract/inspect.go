// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"reflect"
	"strings"

	badger "github.com/dgraph-io/badger"
)

// inspect reports what an archive's index says about each listed key, whose
// value may be unreadable: the version it was written at (Badger's commit
// timestamp, which orders writes in time), whether the value was stored in the
// index or the log, and where in the log and how long. Tab-separated to stdout.
func inspect(archiveArg, keysPath string) error {
	name, path, _ := strings.Cut(archiveArg, "=")
	db, err := badger.Open(badger.DefaultOptions(path).WithReadOnly(true).WithLogger(nil))
	if err != nil {
		return fmt.Errorf("archive %s: %w", name, err)
	}
	f, err := os.Open(keysPath)
	if err != nil {
		return err
	}
	defer f.Close()
	txn := db.NewTransaction(false)
	defer txn.Discard()
	fmt.Println("key\tversion\tmeta\tvlog\toffset\tlength\tvalue")
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, err := hex.DecodeString(strings.Fields(sc.Text())[0])
		if err != nil {
			return err
		}
		item, err := txn.Get(k)
		if err != nil {
			fmt.Printf("%x\t-\t-\t-\t-\t%v\n", k, err)
			continue
		}
		rv := reflect.ValueOf(item).Elem()
		meta := byte(rv.FieldByName("meta").Uint())
		vptr := rv.FieldByName("vptr").Bytes()
		v, _ := item.ValueCopy(nil)
		if len(v) > 16 {
			v = v[:16]
		}
		if meta&bitValuePointer == 0 || len(vptr) < 12 {
			fmt.Printf("%x\t%d\t%x\tinline\t-\t%d\t%x\n", k, item.Version(), meta, len(vptr), v)
			continue
		}
		fmt.Printf("%x\t%d\t%x\t%d\t%d\t%d\t%x\n", k, item.Version(), meta,
			binary.BigEndian.Uint32(vptr[0:4]), binary.BigEndian.Uint32(vptr[8:12]), binary.BigEndian.Uint32(vptr[4:8]), v)
	}
	return sc.Err()
}
