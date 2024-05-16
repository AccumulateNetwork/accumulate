package blockchainDB

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/stretchr/testify/assert"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
)

const (
	Type          = BFilePerm // type specifies stuff like perm, scratch, etc.
	Partition     = BFileDN   // Partition (DN, BVN0, BVN1, etc.)
	Writes        = 10_000    // Total writes in the test
	DBBlockWrites = 1_000_000 // Number of writes in a DBBlock
	MaxSize       = 256       // Max size of a value
	MinSize       = 128       // Minimum size of a value
	MaxBadgerSize = 10_000    // Maximum size of Badger tx
)

var Directory = filepath.Join(os.TempDir(), "DBBlock")
var KeySliceDir = filepath.Join(Directory, "keySlice")

func TestWriteSmallKeys(t *testing.T) {

	filename := filepath.Join(os.TempDir(), "BFileTest.dat")
	bFile, err := NewBFile( filename,5)
	assert.NoError(t, err, "expected no error creating BBFile")

	getKey := func(v byte) (r [32]byte) {
		for i := range r {
			r[i] = v
		}
		return r
	}
	for i := 0; i < Writes; i++ { // For numKeys
		key := getKey(byte(i))

		err := bFile.Put(key, key[:])
		assert.NoError(t, err, "put on BFile fail")
	}
	bFile.Close()
	bFile.Block()

}

func TestWriteKeys(t *testing.T) {
	os.RemoveAll(Directory)
	os.Mkdir(Directory, os.ModePerm)

	start := time.Now()
	filename := filepath.Join(os.TempDir(), "BFileTest.dat")
	bFile, err := NewBFile(filename,5)
	assert.NoError(t, err, "expected no error creating BBFile")

	fr := NewFastRandom([32]byte{}) // Make a random number generator
	for i := 0; i < Writes; i++ {   // For numKeys
		key := fr.NextHash()                   // Get a key.    This generates the same keys
		value := fr.RandBuff(MinSize, MaxSize) // Get a value   and values every time

		err := bFile.Put(key, value)
		assert.NoError(t, err, "put on BFile fail")
	}
	// Close the bFile (writes out the keys)
	bFile.Close() // Close the bFile
	bFile.Block() // Wait for all writes/close to complete.

	fmt.Printf("Writing %d key/values took %v\n", Writes, time.Since(start))
}

func TestReadKeys(t *testing.T) {
	os.RemoveAll(Directory)
	os.Mkdir(Directory, os.ModePerm)

	filename := filepath.Join(os.TempDir(), "BFileTest.dat")
	bFile, err := NewBFile(filename,5)
	assert.NoError(t, err, "expected no error creating BBFile")

	fr := NewFastRandom([32]byte{}) // Make a random number generator
	for i := 0; i < Writes; i++ {   // For numKeys
		key := fr.NextHash()                   // Get a key.    This generates the same keys
		value := fr.RandBuff(MinSize, MaxSize) // Get a value   and values every time

		err := bFile.Put(key, value)
		assert.NoError(t, err, "put on BFile fail")
	}
	// Close the bFile (writes out the keys)
	bFile.Close() // Close the bFile
	bFile.Block() // Wait for all writes/close to complete.

	// Open the bFile for read, and check the keys in it
	bFile, err = OpenBFile(5, filename)
	assert.NoError(t, err, "failed to open BFile for read")

	fr = NewFastRandom([32]byte{}) // reset the random number generator
	for i := 0; i < Writes; i++ {  // For numKeys
		key := fr.NextHash()                   // Get a key.    This generates the same keys
		value := fr.RandBuff(MinSize, MaxSize) // Get a value   and values every time

		v, err := bFile.Get(key)
		assert.NoError(t, err, "Should get all the values back")
		assert.True(t, bytes.Equal(value, v), "Should get back the same value")
	}

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
