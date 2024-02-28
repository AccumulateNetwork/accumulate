package multipleDB

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type KeyFile struct {
	Directory string
	Type      int
	Partition int
	Slice    int
	File      *os.File
	NewKeys   []DBBKeyFull
	Keys      []DBBKeyFull
	Offset    uint64
}

func (kf *KeyFile) Open(slice int) (err error) {
	kf.Slice=slice
	filename := filepath.Join(kf.Directory, fmt.Sprintf("KeyFile_%3d.dat", slice))
	switch {
	case errors.Is(err, os.ErrNotExist):
		if kf.File, err = os.Create(filename); err != nil {
			return err
		}
		kf.Offset = 8
		var offsetB [8]byte // Write an initial offset of 8 to the new file
		offsetB[7] = 8
		kf.File.Write(offsetB[:])
	case err != nil:
		return err
	default:
		if kf.File, err = os.OpenFile(filename, os.O_RDWR, os.ModePerm); err != nil {
			return err
		}
		var offsetB [8]byte
		if _, err = kf.File.Read(offsetB[:]); err != nil {
			kf.File.Close()
			return err
		}
		kf.Offset = binary.BigEndian.Uint64(offsetB[:])
		if _, err = kf.File.Seek(int64(kf.Offset), io.SeekStart); err != nil { // Seek to start of keys.
			kf.File.Close()
			return err
		}
		Keys, err := io.ReadAll(kf.File)
		if err != nil {
			return err
		}
		kf.Keys = make([]DBBKeyFull, len(Keys)/48)
		for i := range kf.Keys {
			copy(kf.Keys[i].Key[:], Keys[:32])
			kf.Keys[i].Offset = binary.BigEndian.Uint64(Keys[32:])
			kf.Keys[i].Length = binary.BigEndian.Uint64(Keys[40:])
			Keys = Keys[48:]
		}
		kf.File.Seek(int64(kf.Offset), io.SeekStart) // Position to add more values
	}
	return nil
}

// Close
// close the KeyFile
func (kf *KeyFile) Close() error {
	
	// Add new keys to existing keys
	n := len(kf.Keys)
	keys := make([]DBBKeyFull,n+len(kf.NewKeys))
	for i := range kf.Keys {
		keys[i]=kf.Keys[i]
	}
	for i,k := range kf.NewKeys {
		keys[n+i]=k
	}

	sort.Slice(kf.Keys,func(i,j int)bool{return bytes.Compare(kf.Keys[i].Key[:],kf.Keys[j].Key[:])<0})
	
	offset,err := kf.File.Seek(0,io.SeekCurrent)
	if err != nil {
		return err
	}
	if _,err = kf.File.Seek(0,io.SeekStart);err != nil {
		return err
	}
	var offsetB [8]byte
	binary.BigEndian.PutUint64(offsetB[:],uint64(offset)) // Update offset to keys
	if _,err = kf.File.Seek(0,io.SeekEnd); err != nil {
		return err
	}
	for _,dbbKey := range kf.Keys {
		kf.File.Write(dbbKey.Bytes())
	}
	return kf.File.Close()
}

// Put
// Add a key value pair to the KeyFile
func (kf *KeyFile) Put (dbKey DBBKeyFull, value []byte) (err error) {
	dbKey.Offset=kf.Offset
	if _, err = kf.File.Write(value); err != nil {
		return err
	}
	kf.Offset+=uint64(len(value))
	kf.NewKeys=append(kf.NewKeys,dbKey)
	return nil
}


func (kf *KeyFile) Update(Directory string, Type int, Partition int) (err error) {

	// Open the BFile with the given Directory, Type, and Partition
	bFile, keys, err := Open(Directory, Type, Partition, 0)
	if err != nil {
		return err
	}

	for slice := 0; slice < 256; slice++ {
		if err = kf.Open(slice); err != nil{
			return err
		}
		for _, dbKey := range keys {
			if dbKey.Key[0] == byte(slice) {
				value, err := bFile.Get(dbKey)
				if err != nil {
					return err
				}
				kf.Put(dbKey,value)
			}
		}
		kf.Close()
	}

	return nil
}
