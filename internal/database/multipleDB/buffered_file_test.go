package multipleDB

import (
	"bytes"
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
	Type          = 0          // type specifies stuff like perm, scratch, etc.
	Partition     = 0          // Partition (DN, BVN0, BVN1, etc.)
	Writes        = 10_000_000 // Total writes in the test
	DBBlockWrites = 1_000_000  // Number of writes in a DBBlock
	MaxSize       = 256        // Max size of a value
	MinSize       = 128        // Minimum size of a value
	MaxBadgerSize = 10_000     // Maximum size of Badger tx
)

var Directory = filepath.Join(os.TempDir(), "DBBlock")
var KeySliceDir = filepath.Join(Directory, "keySlice")

func TestReadKeys(t *testing.T) {
	os.RemoveAll(Directory)
	os.Mkdir(Directory, os.ModePerm)

	const numKeys = 3

	bFile, err := NewBFile(5, Directory, Type, Partition)
	assert.NoError(t, err, "expected no error creating BBFile")

	fr := NewFastRandom([32]byte{}) // Make a random number generator
	for i := 0; i < numKeys; i++ {  // For numKeys
		key := fr.NextHash()                   // Get a key
		value := fr.RandBuff(MinSize, MaxSize) // Get a value

		_, err := bFile.Put(key, value)
		assert.NoError(t, err, "put on BFile fail")
	}
	// Close the bFile (writes out the keys)
	bFile.Close() // Close the bFile
	bFile.Block() // Wait for all writes/close to complete.

	// Open the bFile for read, and check the keys in it
	bFile, err = OpenBFile(Directory, Type, Partition, 0)
	assert.NoError(t, err, "failed to open BFile for read")

	var keys []int
	assert.NoError(t, err, "failed to get keys")
	assert.Equal(t, numKeys, len(keys), "Wrong number of keys written")

	fr = NewFastRandom([32]byte{}) // reset the random number generator
	for i := 0; i < numKeys; i++ { // For numKeys
		key := fr.NextHash()                   // Get a key
		value := fr.RandBuff(MinSize, MaxSize) // Get a value
		v, err := bFile.Get(key)
		assert.NoError(t, err, "Should get all the values back")
		assert.True(t, bytes.Equal(value, v), "Should get back the same value")
	}

}

// keyValue
// Return a random Key Value
func keyValue(fr *FastRandom) (key [32]byte, value []byte) {
	return fr.NextHash(), fr.RandBuff(MinSize, MaxSize)
}

func TestBBFile(t *testing.T) {
	fr := NewFastRandom([32]byte{})
	fmt.Println("TestBBFile Past result: 478825 t/s")

	os.RemoveAll(Directory)
	os.Mkdir(Directory, os.ModePerm)

	bbFile, err := NewBFile(5, Directory, Type, Partition)
	assert.NoError(t, err, "expected no error creating BBFile")
	defer func() {
		if bbFile.File.Close(); err != nil {
			panic(err)
		}
	}()

	start := time.Now()

	j := 0
	for i := 0; i < Writes; i++ {
		if j > DBBlockWrites {
			err := bbFile.Next()
			assert.NoError(t, err, "fail going to next DBBlock")
			j = 0
		}
		j++
		_, err := bbFile.Put(keyValue(fr))
		assert.NoError(t, err, "put on BFile fail")

	}

	fmt.Printf("Total writes: %s Total time: %v --%10.0f t/s\n",
		humanize.Comma(Writes), time.Since(start),
		float64(Writes)/time.Since(start).Seconds())
}

func TestBBFileBadger(t *testing.T) {
	fmt.Println("TestBBFileBadger 1189.5 t/s")

	os.RemoveAll(Directory)
	os.Mkdir(Directory, os.ModePerm)

	bbFile, err := NewBFile(5, Directory, 0, 0)
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
		_, err := bbFile.Put(key, value)
		assert.NoError(t, err, "put on BFile fail") // BROKEN BROKEN
		if txSize+32+64 > MaxBadgerSize {
			err = tx.Commit()
			assert.NoError(t, err, "fail to commit")
			tx.Discard()
			tx = DB.NewTransaction(true)
		}
		//err = tx.Set(key[:], hol[:])
		assert.NoError(t, err, "badger Set fail")
		txSize += 32 + 64 // figure txSize is value + key + overhead. Not exact.

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
