package multipleDB

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dustin/go-humanize"
	"github.com/stretchr/testify/assert"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
)

const (
	Directory     = "/tmp/DBBlockTest"          // Where files go
	KeySliceDir   = "/tmp/DBBlockTest/keySlice" // Where key slices go
	Type          = 0                           // type specifies stuff like perm, scratch, etc.
	Partition     = 0                           // Partition (DN, BVN0, BVN1, etc.)
	Writes        = 100                         // Total writes in the test
	DBBlockWrites = 10                          // Number of writes in a DBBlock
	MaxSize       = 2000                        // Max size of a value
	MinSize       = 200                         // Minimum size of a value
	MaxBadgerSize = 10_000                      // Maximum size of Badger tx
)

func TestReadKeys(t *testing.T) {
	bFile, err := Open(Directory, Type, Partition, 0)
	assert.NoError(t, err, "failed to open DBBlock file")
	keys, err := bFile.GetKeys()
	assert.NoError(t, err, "failed to get keys")
	for i, k := range keys {
		fmt.Printf("%x %8d %8d\n", k.Key, k.Offset, k.Length)
		if i > 10 {
			break
		}
	}
}

func TestBBFile(t *testing.T) {
	fmt.Println("TestBBFile Past result: 587365.7 t/s")

	os.RemoveAll(Directory)
	os.Mkdir(Directory, os.ModePerm)

	bbFile, err := NewBFile(Directory, Type, Partition)
	assert.NoError(t, err, "expected no error creating BBFile")
	defer func() {
		if bbFile.File.Close(); err != nil {
			panic(err)
		}
	}()

	start := time.Now()

	var rh common.RandHash
	data := rh.GetRandBuff(2000)
	j := 0
	for i := 0; i < Writes; i++ {
		if j > DBBlockWrites {
			err := bbFile.Next()
			assert.NoError(t, err, "fail going to next DBBlock")
			j = 0
		}
		j++
		sz := rh.GetIntN(MaxSize-MinSize) + MinSize
		key := rh.NextA()
		_, err := bbFile.Put(key, data[:sz])
		assert.NoError(t, err, "put on BFile fail")

	}

	fmt.Printf("Total writes: %s Total time: %v --%10.1f t/s\n",
		humanize.Comma(Writes), time.Since(start),
		float64(Writes)/time.Since(start).Seconds())
}

func TestBBFileBadger(t *testing.T) {
	fmt.Println("TestBBFileBadger 1189.5 t/s")

	os.RemoveAll(Directory)
	os.Mkdir(Directory, os.ModePerm)

	bbFile, err := NewBFile(Directory, 0, 0)
	assert.NoError(t, err, "expected no error creating BBFile")
	defer func() {
		if bbFile.File.Close(); err != nil {
			panic(err)
		}
	}()

	badgerFile := filepath.Join(Directory, "badger")
	DB, err := badger.Open(badger.DefaultOptions(badgerFile))
	defer func() {
		_ = DB.Close()
	}()
	assert.NoError(t, err, "failed to open badger db")
	tx := DB.NewTransaction(true)
	txSize := 0

	start := time.Now()

	var rh common.RandHash
	j := 0
	for i := 0; i < Writes; i++ {
		if j > DBBlockWrites {
			err := bbFile.Next()
			assert.NoError(t, err, "fail going to next DBBlock")
			j = 0
		}
		j++
		value := rh.GetRandBuff(rh.GetIntN(MaxSize-MinSize) + MinSize)
		key := sha256.Sum256(value)
		hol, err := bbFile.Put(key, value)
		assert.NoError(t, err, "put on BFile fail")
		if txSize+len(hol)+64 > MaxBadgerSize {
			err = tx.Commit()
			assert.NoError(t, err, "fail to commit")
			tx.Discard()
			tx = DB.NewTransaction(true)
		}
		err = tx.Set(key[:], hol[:])
		assert.NoError(t, err, "badger Set fail")
		txSize += len(hol) + 64 // figure txSize is value + key + overhead. Not exact.

	}
	err = tx.Commit()
	assert.NoError(t, err, "fail to commit")
	tx.Discard()

	fmt.Printf("%10.1f t/s", float64(Writes)/time.Since(start).Seconds())
}

func TestBadger(t *testing.T) {
	fmt.Println("TestBadger 1141.7 t/s")

	os.RemoveAll(Directory)
	os.Mkdir(Directory, os.ModePerm)

	badgerFile := filepath.Join(Directory, "badger")
	DB, err := badger.Open(badger.DefaultOptions(badgerFile))
	defer func() {
		_ = DB.Close()
	}()
	assert.NoError(t, err, "failed to open badger db")
	tx := DB.NewTransaction(true)
	txSize := 0

	start := time.Now()

	var rh common.RandHash
	for i := 0; i < Writes; i++ {
		value := rh.GetRandBuff(rh.GetIntN(MaxSize-MinSize) + MinSize)
		key := sha256.Sum256(value)

		if txSize+len(value) > MaxBadgerSize {
			err = tx.Commit()
			assert.NoError(t, err, "fail to commit")
			tx.Discard()
			tx = DB.NewTransaction(true)
		}
		err = tx.Set(key[:], value)
		assert.NoError(t, err, "badger Set fail")
		txSize += len(value) + 64 // figure txSize is value + key + overhead. Not exact.

	}
	err = tx.Commit()
	assert.NoError(t, err, "fail to commit")
	tx.Discard()

	for i := 0; i < Writes; i++ {

	}

	fmt.Printf("%10.1f t/s", float64(Writes)/time.Since(start).Seconds())
}


