package blockchainDB

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

var Directory = filepath.Join(os.TempDir(), "ShardDBTest")
var KeySliceDir = filepath.Join(Directory, "keySlice")

func TestWriteSmallKeys(t *testing.T) {

	filename := filepath.Join(os.TempDir(), "BFileTest.dat")
	bFile, err := NewBFile(filename, 5)
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
	bFile, err := NewBFile(filename, 5)
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
	bFile, err := NewBFile(filename, 5)
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

func TestCompress(t *testing.T) {

	fr := NewFastRandom([32]byte{1, 2, 3, 4, 5}) // Random generator

	TotalKeys := 100000            // Allocate some keys
	Keys := make(map[int][32]byte) // put into a map
	for i := 0; i < TotalKeys; i++ {
		Keys[i] = fr.NextHash()
	}

	TotalCompressions := 10 // Compress this many times after writing
	TotalWrites := 10000  // so many keys

	Directory := filepath.Join(os.TempDir(), "dynamic")
	os.Mkdir(Directory,os.ModePerm)
    defer os.RemoveAll(Directory)
	filename := filepath.Join(Directory, "shard.dat")

	BFile,err := NewBFile(filename, 5)
	assert.NoError(t,err,"failed to create BFile")
	
	for i := 0; i < TotalCompressions; i++ {
		newLen :=0
		for i := 0; i < TotalWrites; i++ {
			key := Keys[int(fr.UintN(uint(TotalKeys)))]
			value := fr.RandBuff(200, 1024)
			err := BFile.Put(key,value)
			newLen += len(value)
			assert.NoError(t,err,"failed to write value")
		}
		fmt.Printf("compress %3d ",i)
		BFile, err = BFile.Compress()
		fmt.Print("!\n")
		if err != nil {
			panic(err)
		}
		assert.NoError(t,err,"failed to compress")
	}

}
