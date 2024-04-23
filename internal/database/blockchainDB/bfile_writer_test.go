package multipleDB

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBFileWriter(t *testing.T) {
	os.RemoveAll(Directory)
	os.Mkdir(Directory, os.ModePerm)
	defer os.RemoveAll(Directory)
	
	const numDataWrites = 20000

	buffPool := make(chan *[BufferSize]byte, 10) // Make the buffPool
	buffPool <- new([BufferSize]byte)            // Use 5 buffers
	buffPool <- new([BufferSize]byte)
	buffPool <- new([BufferSize]byte)
	buffPool <- new([BufferSize]byte)
	buffPool <- new([BufferSize]byte)

	for i := 0; i < 10; i++ { // How many file to be built
		fullPool := len(buffPool)                                                       // Used to check the pool being full later
		buffer := <-buffPool                                                            // Get a buffer to use
		filename := filepath.Join("/tmp", "DBBlockTest", fmt.Sprintf("file-%d.dat", i)) // Compute file name
		file, err := os.Create(filename)                                                // create the file
		assert.NoError(t, err, "could not could not create file "+filename)             // should work
		fw := NewBFileWriter(file, buffPool)                                            // Create a new File Writer

		var data [256]byte    // Compute some data to fill the file with
		for j := range data { // Put the numbers in a buffer
			data[j] = byte(j + i)
		}

		EOB := 8                             // Reserve 8 bytes for the offset
		EOD := uint64(0)                     // Account for that with the end
		for i := 0; i < numDataWrites; i++ { // Write some number of data records
			if EOB+len(data) > BufferSize { //  If txt doesn't fit, write buffer
				fw.Write(buffer, EOB)        // write buffer
				EOD += uint64(EOB)           // Update EOD by what was written
				EOB = 0                      // Reset Buffer to zero for the new buffer
				buffer = <-buffPool          // Get a new buffer
				time.Sleep(time.Microsecond) // Sleep just the shortest amount of time for go routines
			}
			copy(buffer[EOB:], data[:]) // Add the line to the buffer
			EOB += len(data)            // Increment End of Buffer
		}
		allOkay := true
		expectedEOD := EOD + uint64(EOB)   // To be tested later...
		fw.Close(buffer, EOB, expectedEOD) // Close the file, flush current buffer
		for len(buffPool) != fullPool {    // Block until all the buffers come back
			time.Sleep(time.Microsecond)
		}

		file, err = os.Open(filename)                                               // Open the file for reading
		allOkay = allOkay && assert.NoError(t, err, "Failed to open file to check") // Which should work. I was just Created

		var bOffset [8]byte
		file.Read(bOffset[:])
		assert.Equal(t, expectedEOD, binary.BigEndian.Uint64(bOffset[:]), "size not recorded properly")

		for j := 0; j < numDataWrites; j++ {
			data2 := data
			n, err := file.Read(data2[:])
			allOkay = allOkay && assert.NoError(t, err, "error reading the data")
			allOkay = allOkay && assert.Equal(t, n, len(data2), "error not reading enough data")
			allOkay = allOkay && assert.Equalf(t, data, data2, "Data read is not equal on %s %d read", filename, j)

		}
		allOkay = allOkay && assert.NoError(t, err, "no info for buffered file")
		info, _ := file.Stat()
		assert.Equal(t, expectedEOD, uint64(info.Size()))
		file.Close()
		if !allOkay {
			assert.FailNow(t, "failures")
		}
	}
}
