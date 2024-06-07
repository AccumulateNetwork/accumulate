package blockchainDB

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Create and close a BlockFile
func TestCreateBlockFile(t *testing.T) {
	Directory := filepath.Join(os.TempDir(), "BFTest")
	os.RemoveAll(Directory)

	bf, err := NewBlockList(Directory, 1, 5)
	assert.NoError(t, err, "error creating BlockList")
	assert.NotNil(t,bf,"failed to create BlockList")
	bf.Close()

	os.RemoveAll(Directory)
}

func TestOpenBlockFile(t *testing.T) {
	Directory := filepath.Join(os.TempDir(), "BFTest")
	os.RemoveAll(Directory)

	bf, err := NewBlockList(Directory, 1, 5)
	assert.NoError(t, err, "failed to create a BlockFile")
	bf.Close()

	bf, err = NewBlockFile(Directory, 5)
	assert.NoError(t, err, "failed to create a BlockFile")
	bf.Close()

	os.RemoveAll(Directory)
}

func TestBlockFileLoad(t *testing.T) {
	fmt.Println("Testing open and close of a BlockList")
	Directory := filepath.Join(os.TempDir(), "BFTest")
	os.RemoveAll(Directory)
	bf, err := NewBlockList(Directory, 1, 5)
	assert.NoError(t, err, "Failed to create BlockList")
	bf.Close()

	fmt.Printf("Writing BlockFiles: ")

	bf, err = NewBlockFile(Directory, 5)
	assert.NoError(t, err, "failed to create a BlockFile")

	fr := NewFastRandom([32]byte{1, 2, 3})
	for i := 0; i < 2; i++ { // Create so many Blocks
		fmt.Printf("%3d ", i)
		bf.NextBlockFile()
		for i := 0; i < 10; i++ { // Write so many key value pairs
			bf.Put(fr.NextHash(), fr.RandBuff(100, 300))
		}
	}

	fmt.Printf("\nReading BlockFiles: ")

	bf.Close()
	fr = NewFastRandom([32]byte{1, 2, 3})
	for i := 0; i < 2; i++ {
		fmt.Printf("%3d ", i)
		_, err := bf.OpenBList(i, 5)
		assert.NoErrorf(t, err, "failed to open block file %d", i)
		for j := 0; j < 10; j++ {
			hash := fr.NextHash()
			value := fr.RandBuff(100, 300)
			v, err := bf.Get(hash)
			assert.NoError(t, err, "failed to get value for key")
			assert.Equalf(t, value, v, "blk %d pair %d value was not the value expected", j, j)
		}
	}
	fmt.Print("\nDone\n")
}
