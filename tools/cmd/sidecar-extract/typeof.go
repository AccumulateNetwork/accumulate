// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	badger "github.com/dgraph-io/badger"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// typeOf reports, for each 16-hex-char (or full) hash prefix on stdin lines, the
// message and (if a transaction) body type, read from the sidecar-backed
// archives. A hash whose body cannot be read is reported as such.
func typeOf(archiveArgs []string, sidecar, keysPath string) error {
	var copies []*archive
	for _, arg := range archiveArgs {
		n, path, _ := strings.Cut(arg, "=")
		db, err := badger.Open(badger.DefaultOptions(path).WithReadOnly(true).WithLogger(nil))
		if err != nil {
			return fmt.Errorf("archive %s: %w", n, err)
		}
		copies = append(copies, &archive{name: n, db: db, log: &vlog{dir: path}})
	}
	store := &verifiedStore{archives: copies}
	if sidecar != "" {
		side, err := leveldb.OpenFile(sidecar, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
		if err != nil {
			return err
		}
		defer side.Close()
		store.extra = append(store.extra, side)
	}
	batch := coredb.New(store, nil).Begin(false)
	defer batch.Discard()

	f, err := os.Open(keysPath)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	var n int
	for sc.Scan() {
		hs := strings.Fields(sc.Text())[0]
		if len(hs)%2 == 1 {
			hs = hs[:len(hs)-1]
		}
		hb, err := hex.DecodeString(hs)
		if err != nil || len(hb) == 0 {
			fmt.Printf("%s\tbad-hash\n", hs)
			continue
		}
		var h [32]byte
		copy(h[:], hb)
		if n++; n%50000 == 0 {
			batch.Discard()
			batch = coredb.New(store, nil).Begin(false)
		}
		msg, err := batch.Message(h).Main().Get()
		if err != nil {
			fmt.Printf("%s\tunreadable\t%v\n", hs, err)
			continue
		}
		typ := msg.Type().String()
		if tm, ok := msg.(*messaging.TransactionMessage); ok {
			typ += "/" + tm.Transaction.Body.Type().String()
		}
		if sm, ok := msg.(*messaging.SignatureMessage); ok && sm.Signature != nil {
			typ += "/" + sm.Signature.Type().String()
		}
		fmt.Printf("%s\t%s\n", hs, typ)
	}
	return sc.Err()
}
