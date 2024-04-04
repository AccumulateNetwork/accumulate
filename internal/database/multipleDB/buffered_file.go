package multipleDB

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// BFile
// This is a Block buffered file designed to support a write only file
// where data is only appended to the file.
//
// BFile keeps a set of buffers.
//
// Each Buffer:
//    The first 8 bytes points to the end of the data portion of the
//    the file.
//
//    UpdateEndOfData updates the end of data offset
//
//    Key entries follow the EndOfData
//
//    Data is added to the buffer until it is full.
//
//    When the buffer is full, a go routine is created to flush
//    the buffer is written to disk.
//
//    Once the buffer is reset, it is put back into the waiting channel
//
// When the BFile is closed, the first 8 bytes are updated to point
// to the end of the file
//

const BufferSize = 1024 * 1024 * 1 // N MB, i.e. N *(1024^2)

// BBuff
// Block Buff
// Holds all the persisted transactions and their keys.
type BBuff struct {
	Buffer [BufferSize]byte    // Buffer of all the values
	File   *os.File            // File where the buffer is persisted
	EOD    int                 // offset to the end of data (for the last Buffer written to the file)
	Keys   map[[32]byte]DBBKey // Keys and offset to the values the values
}

// Block File
// Holds the buffers and ID stuff needed to build DBBlocks (Database Blocks)
type BFile struct {
	File      *os.File               // The file being buffered
	Keys      map[[32]byte]DBBKey    // The set of keys written to the BFile
	Directory string                 // Directory where the files go
	Type      int                    // Need types: scratch, permanent at least.
	Partition int                    // Partition of the the BFile
	Height    int                    // Height of the BFile
	BuffPool  chan *[BufferSize]byte // Buffer Pool (buffers not in use)
	Buffer    *[BufferSize]byte      // The current buffer under construction
	BufferCnt int                    // Number of buffers used by the bfWriter
	bfWriter  *BFileWriter           // Writes buffers to the File
	EOD       uint64                 // Offset to the end of Data for the whole file(place to hold it until the file is closed)
	EOB       int                    // End of the current buffer... Where to put the next data 'write'
}

// Get
// Get the value for a given DBKeyFull
func (b *BFile) Get(Key [32]byte) (value []byte, err error) {
	dBBKey := b.Keys[Key]
	if _, err = b.File.Seek(int64(dBBKey.Offset), io.SeekStart); err != nil {
		return nil, err
	}
	value = make([]byte, dBBKey.Length)
	_, err = b.File.Read(value)
	return value, err
}

// newBlock
// create a new block.  Expects that the Block Height is updated already.  Leaves room
// for the 8 byte offset to the keys
func (b *BFile) newBlock() (err error) {
	filename := fmt.Sprintf("BBlock_%03d_%02d_%09d.dat", b.Partition, b.Type, b.Height) // Compute the next file name
	if b.File, err = os.Create(filepath.Join(b.Directory, filename)); err != nil {      // Create the new file
		return err
	}
	if b.Buffer == nil {
		b.Buffer = <-b.BuffPool
	}
	b.bfWriter = NewBFileWriter(b.File, b.BuffPool)
	b.EOB = 0
	b.EOD = 8
	return nil
}

// Close
// Closes the BFile, and flushes any buffer to disk.  All buffers remain in the
// buffer pool.
func (b *BFile) Close() {
	if b.bfWriter != nil {
		eod := b.EOD // Keep the current EOD so we can close the BFile properly with an offset to the keys

		keys := make([][48]byte, len(b.Keys)) // Collect all the keys into a list to sort them
		i := 0                                // This ensures that all users get the same DBBlocks
		for k, v := range b.Keys {            // Since maps randomize order
			keys[i] = [48]byte(v.Bytes(k)) //    Copy each key into a 48 byte entry
		}
		b.Keys = make(map[[32]byte]DBBKey) //    Once we have the list of keys, we don't need the map anymore

		// Sort all the entries by the keys.  Because no key will be a duplicate, it doesn't matter
		// that the offset and length are at the end of the 48 byte entry
		sort.Slice(keys, func(i, j int) bool { return bytes.Compare(keys[i][:], keys[j][:]) < 0 })

		for _, k := range keys { // Once keys are sorted, write all the keys to the end of the DBBlock
			err := b.Write(k[:]) //
			if err != nil {
				panic(err)
			}
		}
		b.bfWriter.Close(b.Buffer, b.EOB, eod) // Close that file
		b.bfWriter = nil                       // kill any reference to the bfWriter
		b.Buffer = nil                         // Close writes the buffer, and the file is closed. clear the buffer
	}
}

// OpenBFile
// Open a DBBlock file at a given height for read access only
func OpenBFile(Directory string, Type int, Partition int, Height int) (bFile *BFile, err error) {
	bFile = new(BFile)          // create a new BFile
	bFile.Directory = Directory // Set Directory
	bFile.Type = Type           // Type is like perm, scratch, etc.
	bFile.Partition = Partition // Set Partition
	bFile.Height = Height - 1   // OpenNext is going to increment the Height; adjust
	return bFile.OpenNext()
}

// OpenNext
// Open next DBBlock for reading only
func (b *BFile) OpenNext() (bFile *BFile, err error) {

	b.Height++ // Go to the next DBBlock

	filename := fmt.Sprintf("BBlock_%03d_%02d_%09d.dat", b.Partition, b.Type, b.Height) // Compute the next file name
	if b.File, err = os.Open(filepath.Join(b.Directory, filename)); err != nil {
		return nil, err
	}

	var offsetB [8]byte
	if _, err := b.File.Read(offsetB[:]); err != nil {
		return nil, err
	}
	if newOffset, err := b.File.Seek(int64(binary.BigEndian.Uint64(offsetB[:])), io.SeekStart); err != nil {
		return nil, err
	} else {
		_ = newOffset
	}

	// Load all the keys into the map
	keyList, err := io.ReadAll(b.File)
	for i := 0; i < len(keyList)/48; i++ {
		dbBKey := new(DBBKey)
		address, err := dbBKey.Unmarshal(keyList)
		if err != nil {
			return nil, err
		}
		b.Keys[address] = *dbBKey
		keyList = keyList[48:]
	}

	return b, err
}

// Block
// Block waits until all buffers have been returned to the BufferPool.
func (b *BFile) Block() {
	for len(b.BuffPool) < b.BufferCnt {
		time.Sleep(time.Microsecond * 50)
	}
}

// NewBFile
// Creates a new Buffered file.  The caller is responsible for writing the header
func NewBFile(BufferCnt int, Directory string, Type int, Partition int) (*BFile, error) {
	if len(Directory) == 0 {
		return nil, fmt.Errorf("must have a Directory")
	}
	bFile := new(BFile)         // create a new BFile
	bFile.Directory = Directory // Set Directory
	bFile.Type = Type           // Type is like perm, scratch, etc.  Could divid up what is stored further
	bFile.Partition = Partition // Set Partition
	bFile.Height = 0            //

	bFile.Keys = make(map[[32]byte]DBBKey)                   // Allocate the Keys amp
	bFile.BufferCnt = BufferCnt                              // How many buffers we are going to use
	bFile.BuffPool = make(chan *[BufferSize]byte, BufferCnt) // Create the waiting channel
	for i := 0; i < BufferCnt; i++ {
		bFile.BuffPool <- new([BufferSize]byte) // Put some buffers in the waiting queue
	}

	if err := bFile.newBlock(); err != nil { // Allocate the buffers
		return nil, err
	}

	bFile.Buffer = <-bFile.BuffPool // Get the first buffer

	var offsetB [8]byte
	if _, err := bFile.File.Write(offsetB[:]); err != nil {
		return nil, err
	}

	bFile.EOB = 8
	bFile.EOD = 8
	return bFile, nil
}

// space
// Returns the number of bytes to the end of the current buffer
func (b *BFile) space() int {
	return BufferSize - b.EOB
}

// Put
// Put a key value pair into the BFile, return the *DBBKeyFull
func (b *BFile) Put(Key [32]byte, Value []byte) (dbbKeyFull *DBBKey, err error) {
	dbbKey := new(DBBKey)
	dbbKey.Offset = b.EOD
	dbbKey.Length = uint64(len(Value))

	b.Keys[Key] = *dbbKey
	err = b.Write(Value)
	return dbbKey, err
}

// Write
// Writes given Data into the BFile onto the End of the BFile.
// The data is copied into a buffer.  If the buffer is full, it is flushed
// to disk.  Left over data goes into the next buffer.
// EOB and EOD are updated as needed.
func (b *BFile) Write(Data []byte) error {

	if b.Buffer == nil { // Get a buffer if it is needed
		b.Buffer = <-b.BuffPool
	}

	space := b.space()
	// Write to the current buffer
	if len(Data) < space {
		copy(b.Buffer[b.EOB:], Data)
		b.EOB += len(Data)
		b.EOD += uint64(len(Data))
		return nil
	}

	if space > 0 {
		copy(b.Buffer[b.EOB:], Data[:b.space()]) // Copy what fits into the current buffer
		b.EOB += space                           // Update b.EOB (should be equal to BufferSize)
	}

	// Write out the current buffer, get the other buffer, and put the rest of Value there.
	b.bfWriter.Write(b.Buffer, b.EOB) // Write out this buffer
	b.Buffer = <-b.BuffPool           // Get the next buffer
	copy(b.Buffer[:], Data[space:])   // Put the rest of the Value into the buffer
	b.EOB = len(Data[space:])         // EOB points to the end of the data written
	b.EOD += uint64(len(Data))
	return nil
}

// Next
// Close out one BFile, and move to the next.
func (b *BFile) Next() (err error) {
	b.Close()
	b.Height++
	return b.newBlock() // Create the new DBBlock file and start grabbing key value pairs again!
}
