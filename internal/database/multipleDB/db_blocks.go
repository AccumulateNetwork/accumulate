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

type DBBKey struct {
	offset int64
	length int64
}

type DBBlock struct {
	Directory string
	Partition int64
	Height    int64
	Keys      map[[32]byte]DBBKey
	Offset    int64
	File      *os.File
}

// NewDBBlock
// Create a new DBBlock for a fresh new database
func NewDBBlock(directory string, partition int64) (dbb *DBBlock, err error) {
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

// Open
// Create a new Block.  Will wipe away an existing block
func (d *DBBlock) Open() (err error) {
	filename := fmt.Sprintf("%03d-%06d.dat",d.Partition,d.Height)
	d.File,err = os.Create(fmt.Sprintf(filepath.Join(d.Directory,filename)))
	if err != nil {
		return err
	}
	var offsetToKeys [8]byte
	_,err = d.File.Write(offsetToKeys[:])
	if err != nil {
		return err
	}

	d.Offset = int64(len(offsetToKeys))
	d.Keys = make(map[[32]byte]DBBKey)
	return nil
}

// Flush
// Writes the DBBlock to disk, and closes the file.  You can't flush
// a block twice.
func (d *DBBlock) Flush() (err error) {

	d.File.Seek(0, io.SeekStart)                            // Go to start of file, write offset to current EOF (start of keys)
	var offset [8]byte                                      // The offset is 8 bytes
	binary.BigEndian.PutUint64(offset[:], uint64(d.Offset)) //
	if _, err := d.File.Write(offset[:]); err != nil {      //
		return err
	}
	d.File.Seek(0, io.SeekEnd)

	list := make([][32]byte, len(d.Keys))
	i := 0
	for k := range d.Keys {
		list[i] = k
		i++
	}
	sort.Slice(list, func(i, j int) bool { return bytes.Compare(list[i][:], list[j][:]) < 0 })
	var buff [48]byte
	for _, k := range list {
		copy(buff[:], k[:])
		key := d.Keys[k]
		binary.BigEndian.PutUint64(buff[32:], uint64(key.offset))
		binary.BigEndian.PutUint64(buff[32:], uint64(key.length))
		if _, err = d.File.Write(buff[:]); err != nil {
			return err
		}
	}

	err = d.File.Close() // Close the file
	return err
}

// Write
// Writes a key value to a DBBlock
func (d *DBBlock) Write(key [32]byte, value []byte) (err error) {
	k := new(DBBKey)
	k.length = int64(len(value))
	k.length = d.Offset
	_, err = d.File.Write(value)
	if err != nil {
		_, err1 := d.File.Seek(d.Offset,io.SeekStart)
		if err1 != nil {
			return fmt.Errorf("error writing %v, error seeking %v",err,err1)
		}
		return err
	}
	return nil
}