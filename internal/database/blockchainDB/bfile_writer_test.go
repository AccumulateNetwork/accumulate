package blockchainDB

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBFileWriter(t *testing.T) {

	const (
		bufferCnt      = 5  // Count of buffers in the buffer pool
		buffersPerFile = 7  // How many buffers written to each file.
		fileCnt        = 11 // How many files
	)

	buffPool := make(chan *[BufferSize]byte, bufferCnt) // Make the buffPool
	for i := 0; i < bufferCnt; i++ {
		buffPool <- new([BufferSize]byte) // Use 5 buffers
	}

	Directory := filepath.Join(os.TempDir(), "bfwFiles")
	os.Mkdir(Directory, os.ModePerm)
	defer os.RemoveAll(Directory)
	
	fw := NewFastRandom([32]byte{2, 3, 4})
	fileSize := uint64(BufferSize * buffersPerFile)

	for i := 0; i < fileCnt; i++ { // How many file to be built
		filename := filepath.Join(Directory, fmt.Sprintf("file%04d.dat", i))
		file, err := os.Create(filename)
		assert.NoError(t, err, "could not create file %s", filename)
		bfw := NewBFileWriter(file, buffPool)
		for i := 0; i < buffersPerFile; i++ {
			b := <-buffPool
			buff := fw.RandBuff(BufferSize, BufferSize)
			copy(b[:], buff)
			bfw.Write(b, BufferSize)
		}
		b := <-buffPool
		bfw.Close(b, 0, fileSize)
	}

	fw = NewFastRandom([32]byte{2, 3, 4})
	var buff [BufferSize]byte

	for i := 0; i < fileCnt; i++ { // How many file to be built
		filename := filepath.Join(Directory, fmt.Sprintf("file%04d.dat", i))
		file, err := os.Open(filename)
		assert.NoError(t, err, "could not create file %s", filename)

		for i := 0; i < buffersPerFile; i++ {
			buf := fw.RandBuff(BufferSize, BufferSize)
			if i == 0 {
				binary.BigEndian.PutUint64(buf, fileSize)
			}
			file.Read(buff[:])
			assert.True(t, bytes.Equal(buf, buff[:]), "data mismatch")
		}
		file.Close()
	}
}
