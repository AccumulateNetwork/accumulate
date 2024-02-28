package multipleDB

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

const BufferSize = 1024 * 1024 * 10 // N MB, i.e. N *(1024^2)

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
	Waiting   chan *[BufferSize]byte // Buffers waiting to be be used
	Buffer    *[BufferSize]byte      // The current buffer under construction
	EOD       uint64                 // Offset to the end of Data for the whole file(place to hold it until the file is closed)
	EOB       int                    // End of the current buffer... Where to put the next data 'write'
}

// Get
// Get the value for a given DBKeyFull
func (b *BFile) Get(Key DBBKeyFull) ( value []byte, err error){
	if _, err = b.File.Seek(int64(Key.Offset),io.SeekStart); err != nil {
		return nil, err
	}
	value = make([]byte,Key.Length)
	_,err = b.File.Read(value)
	return value, err
}

// newBlock
// create a new block.  The header for the first block must be written by caller
func (b *BFile) newBlock() (err error) {
	filename := fmt.Sprintf("BBlock_%3d_%2d_%09d.dat", b.Partition, b.Type, b.Height) // Compute the next file name
	block := <-b.Waiting                                                              // wait for any pending buffer write to finish
	defer func() { b.Waiting <- block }()                                             // then just put the block back into waiting
	if b.File, err = os.Create(filepath.Join(b.Directory, filename)); err != nil {    // Create the new file
		return err
	}
	b.Height++
	b.EOB = 0
	return nil
}

// Open
// Open a DBBlock file at a given height.
// OpenBlock assumes Memory is available to hold 2 copies of the keys in a DBBlock
func Open(Directory string, Type int, Partition int, Height int) (bFile *BFile, keys []DBBKeyFull, err error) {
	bFile = new(BFile)          // create a new BFile
	bFile.Directory = Directory // Set Directory
	bFile.Type = Type           // Type is like perm, scratch, etc.
	bFile.Partition = Partition // Set Partition
	bFile.Height = Height - 1   // OpenNext is going to increment the Height; adjust
	bFile.Close()
	return bFile.OpenNext()
}

// Close
// Close the underlying file object;  The file might not be open, so don't
// bother to return an error on close.
func (b *BFile) Close() {
	b.File.Close()
}

// OpenNext
// Open the next DBBlock.  OpenNext assumes memory is available to hold 2 copies of the keys in a DBBlock
func (b *BFile) OpenNext() (bFile *BFile, keys []DBBKeyFull, err error) {

	bFile.Height++ // Go to the next DBBlock

	filename := fmt.Sprintf("BBlock_%3d_%2d_%09d.dat", b.Partition, b.Type, b.Height) // Compute the next file name
	if b.File, err = os.Open(filepath.Join(b.Directory, filename)); err != nil {
		return nil, nil, err
	}

	var offsetB [8]byte                               // First 8 bytes of DBBlock is the offset to the keys
	if _, err = b.File.Read(offsetB[:]); err != nil { // Load that offset.
		return nil, nil, err
	}
	keyOffset := binary.BigEndian.Uint64(offsetB[:])                      // Seek to start of the keys
	if _, err = b.File.Seek(int64(keyOffset), io.SeekStart); err != nil { //
		return nil, nil, err
	}

	buff, err := io.ReadAll(b.File) // Load all the keys
	if err != nil {
		return nil, nil, err
	}
	keys = make([]DBBKeyFull, len(buff)/48) // Make a slice of all the Full (including address) DBBkeys
	for i := range keys {                   // Walk through all the keys in the buff
		copy(keys[i].Key[:32], buff[:32])                   // Copy the address
		keys[i].Offset = binary.BigEndian.Uint64(buff[32:]) // Get the offset
		keys[i].Length = binary.BigEndian.Uint64(buff[40:]) // Get the length
		buff = buff[48:]                                    // Next 48 bytes in buff
	}

	return bFile, keys, nil
}

// NewBFile
// Creates a new Buffered file.  The caller is responsible for writing the header
func NewBFile(Directory string, Type int, Partition int) (*BFile, error) {
	if len(Directory) == 0 {
		return nil, fmt.Errorf("Must have a Directory")
	}
	bFile := new(BFile)         // create a new BFile
	bFile.Directory = Directory // Set Directory
	bFile.Type = Type           // Type is like perm, scratch, etc.  Could divid up what is stored further
	bFile.Partition = Partition // Set Partition
	bFile.Height = 0            //

	bFile.Keys = make(map[[32]byte]DBBKey)          // Allocate the Keys amp
	bFile.Waiting = make(chan *[BufferSize]byte, 1) // Create the waiting channel
	bFile.Waiting <- new([BufferSize]byte)          // Put a buffer into waiting
	bFile.Buffer = new([BufferSize]byte)            // Create a current buffer

	if err := bFile.newBlock(); err != nil {
		return nil, err
	}

	bFile.EOB = 8

	return bFile, nil
}

// space
// Returns the number of bytes to the end of the current buffer
func (b *BFile) space() int {
	return BufferSize - b.EOB
}

// Put
// Put a key value pair into the BFile, return the key
func (b *BFile) Put(Key [32]byte, Value []byte) (HeightOffsetLength [24]byte, err error) {
	length := uint64(len(Value))
	offset := uint64(b.EOD)
	height := uint64(b.Height)

	binary.BigEndian.PutUint64(HeightOffsetLength[:], height)
	binary.BigEndian.PutUint64(HeightOffsetLength[8:], offset)
	binary.BigEndian.PutUint64(HeightOffsetLength[16:], length)

	b.Keys[Key] = DBBKey{Length: length, Offset: offset}
	err = b.Write(Value)
	return HeightOffsetLength, err
}

func (b *BFile) Write(Value []byte) error {

	b.EOD += uint64(len(Value))

	space := b.space()
	// Write to the current buffer
	if len(Value) < space {
		copy(b.Buffer[b.EOB:], Value)
		b.EOB += len(Value)
		return nil
	}

	if space > 0 {
		copy(b.Buffer[b.EOB:], Value[:b.space()]) // Copy what fits into the current buffer
		b.EOB += space                            // Update b.EOB (should be equal to BufferSize)
	}

	// Write out the current buffer, get the other buffer, and put the rest of Value there.
	buffer := b.Buffer                // Keep a pointer to the current buffer to swap with waiting buffer
	b.Buffer = <-b.Waiting            // If writing is not complete, this is going to block
	go b.Flush(b.File, b.EOB, buffer) // Write out the old buffer, while we build the new buffer
	copy(b.Buffer[:], Value[space:])  // Put the rest of the Value into the buffer
	b.EOB = len(Value[space:])        // EOB points to the end of the data written
	return nil
}

// Flush
// Flush the current buffer out to the BFile. Once the write is complete, put the buffer
// back into the waiting channel.  This ensures that we don't start building on
// a buffer that has not been written to disk yet.
func (b *BFile) Flush(file *os.File, EOB int, buffer *[BufferSize]byte) {
	if _, err := file.Write(buffer[:EOB]); err != nil {
		panic(err)
	}
	b.Waiting <- buffer
}

// Next
// Close out one BFile, and move to the next.
func (b *BFile) Next() (err error) {

	eod := b.EOD // Save the date, er... spot for beginning of keys

	type keyEntry [48]byte

	keys := make([]keyEntry, len(b.Keys)) // Collect all the keys into a list to sort them
	i := 0                                // This ensures that all users get the same DBBlocks
	for k, v := range b.Keys {            // Since maps randomize order
		copy(keys[i][:], k[:])                             // Copy each key into a 48 byte entry
		binary.BigEndian.PutUint64(keys[i][32:], v.Offset) // with the offset
		binary.BigEndian.PutUint64(keys[i][40:], v.Length) // and with the value length
	}
	b.Keys = make(map[[32]byte]DBBKey) //    Once we have the list of keys, we don't need the map anymore

	// Sort all the entries by the keys.  Because no key will be a duplicate, it doesn't matter
	// that the offset and length are at the end of the 48 byte entry
	sort.Slice(keys, func(i, j int) bool { return bytes.Compare(keys[i][:], keys[j][:]) < 0 })

	for _, ke := range keys { // Once keys are sorted, write all the keys to the end of the DBBlock
		b.Write(ke[:]) //        Use the same writing mechanism as the data values
	}

	buffer := b.Buffer             // Keep a pointer to the current buffer to swap with waiting buffer
	b.Buffer = <-b.Waiting         // If writing is not complete, this is going to block
	b.Flush(b.File, b.EOB, buffer) // Write out the old buffer.  Now file has all values and keys
	b.EOB = 0                      // reset pointer to start of block

	if _, err = b.File.Seek(0, io.SeekStart); err != nil { // Seek to start
		return err
	}
	var buff [8]byte                                //        Write out the offset to the keys into
	binary.BigEndian.PutUint64(buff[:], eod)        //         the DBBlock file.
	if _, err = b.File.Write(buff[:]); err != nil { //
		return err
	}

	if err = b.File.Close(); err != nil { // Close the file
		return err
	}

	err = b.newBlock() // Create the new DBBlock file and start grabbing key value pairs again!
	return err
}
