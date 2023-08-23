package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"

	badger "github.com/dgraph-io/badger/v3"
	"github.com/fatih/color"
)

func TestDbRepair(t *testing.T) {
	color.NoColor = false
	dir := t.TempDir()
	goodDB := filepath.Join(dir, "good.db")
	badDB := filepath.Join(dir, "bad.db")
	summaryF := filepath.Join(dir, "summary.dat")
	diffF := filepath.Join(dir, "diff.dat")
	fixF := filepath.Join(dir, "fix.dat")
	buildTestDBs(2e4, goodDB, badDB)
	buildSummary(goodDB, summaryF)
	buildDiff(summaryF, badDB, diffF)
	buildFix(diffF, goodDB, fixF)
	applyFix(fixF, badDB)
	buildDiff(summaryF, badDB, diffF)
}

// Sanity check of what we are doing with databases
func TestCompareDB(t *testing.T) {
	dir := t.TempDir()
	goodDB := filepath.Join(dir, "good.db")
	badDB := filepath.Join(dir, "bad.db")
	summaryF := filepath.Join(dir, "summary.dat")

	buildTestDBs(2e4, goodDB, badDB)
	buildSummary(goodDB, summaryF)

	gDB, gClose := OpenDB(goodDB)
	defer gClose()

	bDB, bClose := OpenDB(badDB)
	defer bClose()

	readAll := func(db *badger.DB) map[[32]byte][]byte {
		keyValue := make(map[[32]byte][]byte)
		err := db.View(func(txn *badger.Txn) error {
			opts := badger.DefaultIteratorOptions
			opts.PrefetchValues = false // <= What this does is go through the keys in the db
			it := txn.NewIterator(opts) //    in whatever order is best for badger for speed
			defer it.Close()
			for it.Rewind(); it.Valid(); it.Next() {
				item := it.Item()
				k := item.Key()        //   Get the key and hash it.  IF the key isn't a
				kh := sha256.Sum256(k) //   hash, then this takes care of that case.
				err := item.Value(func(val []byte) error {
					value := make([]byte, len(val))
					copy(value, val)
					keyValue[kh] = value
					return nil
				})
				checkf(err, "on read of value")
			}
			return nil
		})
		checkf(err, "on viewing keys")
		return keyValue
	}

	good := readAll(gDB)
	bad := readAll(bDB)

	fmt.Printf("Comparing good %d with bad %d\n", len(good), len(bad))

	var cntMod, cntDel, cntAdd int

	for k, v := range good {
		bv, exists := bad[k]
		switch {
		case !exists:
			cntDel++
		case !bytes.Equal(bv, v):
			cntMod++
		}
	}
	for k, _ := range bad {
		_, exists := good[k]
		if !exists {
			cntAdd++
		}
	}

	fmt.Printf("Modified: %d Delete: %d Added: %d\n", cntMod, cntDel, cntAdd)

}
