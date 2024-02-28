package multipleDB

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/edsrzf/mmap-go"
)

type DBBKey struct {
	Offset uint64
	Length uint64
}
type DBBKeyFull struct {
	Key    [32]byte
	Offset uint64
	Length uint64
}

func (d *DBBKeyFull) Bytes()[]byte {
	var buff [48]byte
	copy(buff[:32],d.Key[:])
	binary.BigEndian.PutUint64(buff[32:],d.Offset)
	binary.BigEndian.PutUint64(buff[40:],d.Length)
	return buff[:]
}

type DBBlocks struct {
	keys      mmap.MMap
	Partition uint64
	Height    uint64
	Data      mmap.MMap
}

type DBBlock struct {
	Directory string
	Partition uint64
	Height    uint64
	Data      mmap.MMap
	Keys      map[[32]byte]*DBBKey
	Offset    uint64
	File      *os.File
}

// NewDBBlock
// Create a new DBBlock for a fresh new database
func NewDBBlock(directory string, partition uint64) (dbb *DBBlock, err error) {
	_, err = os.Stat(directory)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(directory, os.ModePerm); err != nil {
			return nil, err
		}
	}

	dbb = new(DBBlock)
	dbb.Directory = directory
	dbb.Partition = partition
	dbb.Height = 0

	return dbb, nil
}

// Close
// Close an open DBBlock.  Really just a flush of the file
func (d *DBBlock) Close() error {
	return d.Flush()
}

// Open
// Create a new Block.
//
//	write == true  : DBBlock will be reset, contents cleared
//	write == false : File opened, keys read
func (d *DBBlock) Open(write bool) (err error) {
	filename := fmt.Sprintf("%03d-%06d.dat", d.Partition, d.Height)

	// if opening the DBBlock for write

	if write {
		// Create the file (delete if it exists)
		if d.File, err = os.Create(filepath.Join(d.Directory, filename)); err != nil {
			return err
		}

		// The first 8 bytes is an offset in the DBBlock to the keys
		// The partition and height are included in the header to make Opening a DBBlock easier
		var offsetToKeys [24]byte                                 // DBBlock Header includes offset to keys (written on close of DBBlock)
		binary.BigEndian.PutUint64(offsetToKeys[8:], d.Partition) // Write Partition number
		binary.BigEndian.PutUint64(offsetToKeys[16:], d.Height)   // Write the Height
		if _, err = d.File.Write(offsetToKeys[:]); err != nil {
			return err
		}

		d.Offset = uint64(len(offsetToKeys)) // Track the offset to the keys as the DBBlock is built
		d.Keys = make(map[[32]byte]*DBBKey)  // While building, keep keys in a map.  Could keep in file, but would have to move
		return nil                           // the keys to end of the DBBlock when closing the DBBlock
	}

	// If opening the DBBlock for read

	if d.File, err = os.Open(filepath.Join(d.Directory, filename)); err != nil { // Open the DBBlock File
		return err //                                                               Error if fails
	}
	var offsetB [8]byte                                 // Restore the offset in the DBBlock to that of the
	if _, err2 := d.File.Read(offsetB[:]); err != nil { // the keys in the DBBlock
		return err2
	}
	d.Offset = binary.BigEndian.Uint64(offsetB[:])                       // Set the offset to start of keys
	if _, err = d.File.Seek(int64(d.Offset), io.SeekStart); err != nil { // Seek to start of keys
		return err
	}

	info, err := d.File.Stat() // Get the eof from the DBBlock
	if err != nil {
		return err
	}
	eof := info.Size() //         Don't read past the EOF

	for int64(d.Offset) < eof {
		newKey := new(DBBKey)                              // Get a new DBBKey
		var key [32]byte                                   // A 32 byte array
		var AKey [48]byte                                  // A buffer for offset and length
		d.File.Read(AKey[:])                               // Read the whole key and the offset/length
		copy(key[:], AKey[:32])                            // Get the key
		newKey.Offset = binary.BigEndian.Uint64(AKey[32:]) // pull the offset
		newKey.Length = binary.BigEndian.Uint64(AKey[40:]) // pull the length
		d.Keys[key] = newKey                               // Restore the key
		d.Offset += 48                                     // update the offset by the length of the key entry
	}

	return nil
}

// Next
// Close the current DBBlock, and open the next DBBlock.
// Note if you are writing, you are overwriting (possibly) existing DBBlock
// If not writing, then open the next DBBlock.  Return os.ErrNotExist when there
// is not a Next DBBlock
func (d *DBBlock) Next(write bool) (err error) {
	if err = d.Flush(); err != nil {
		return err
	}
	d.Height++
	if err = d.Open(write); err != nil {
		return err
	}
	return nil
}

// Flush
// Writes the DBBlock to disk, and closes the file.  You can't flush
// a block twice. DBBlock format for M entries pointing in total to N bytes of values
//
//		8 bytes  -- offset (N+24) to key list
//	    8 bytes  -- partition
//	    8 bytes  -- DBBlock height
//
//	    N bytes  -- values, each written end to end
//
//	    M Keys (48 bytes each)
//	      Key [32]byte
//	      offset [8]byte Big Endian (offset in DBBlock to the value)
//	      length [8]byte Big Endian (length of the value)
func (d *DBBlock) Flush() (err error) {

	d.File.Seek(0, io.SeekStart)                            // Go to start of file, write offset to current EOF (start of keys)
	var offset [8]byte                                      // The offset is 8 bytes
	binary.BigEndian.PutUint64(offset[:], uint64(d.Offset)) //
	if _, err := d.File.Write(offset[:]); err != nil {      // Should not happen
		return err
	}
	d.File.Seek(0, io.SeekEnd) // Put the file pointer to the end of file

	var buff [48]byte // Each key entry is a key (32 bytes), offset (8 bytes), and length (8 bytes)
	for k, v := range d.Keys {
		copy(buff[:], k[:])                             // Put the key in the buff
		binary.BigEndian.PutUint64(buff[32:], v.Offset) // Put the offset in buff
		binary.BigEndian.PutUint64(buff[40:], v.Length) // Put the length in buff
		if _, err = d.File.Write(buff[:]); err != nil { // Write out the key
			return err
		}
	}

	err = d.File.Close() // Close the file
	return err
}

// Write
// Writes a key value to a DBBlock
func (d *DBBlock) Write(key [32]byte, value []byte) (err error) {
	k := new(DBBKey) // Build a key
	k.Offset = d.Offset
	k.Length = uint64(len(value))
	d.Offset += k.Length

	d.Keys[key] = k // Keep the key in the DBBlock Keys map

	if _, err = d.File.Write(value); err != nil { // Write the value out to the DBBlock
		return err //                                For now, we are in trouble if this write fails.
	}

	return nil
}

// Read
// Return the data associated with a key in the DBBlock
func (d *DBBlock) Read(key [32]byte) (value []byte, err error) {

	// Get the key from the DBBlock
	dbBKey, ok := d.Keys[key]
	if !ok {
		return nil, fmt.Errorf("key %x undefined", key)
	}

	// Seek to where the data is in the DBBlock
	if _, err = d.File.Seek(int64(dbBKey.Offset), io.SeekStart); err != nil {
		return nil, err
	}

	// Read the data
	value = make([]byte, dbBKey.Length)
	_, err = d.File.Read(value)
	return value, err // Return whatever error Read might throw
}
