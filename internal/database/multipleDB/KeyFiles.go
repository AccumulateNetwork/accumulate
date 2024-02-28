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

type KeySlice struct {
	Directory string
	slice     int
	File      *os.File
	NewKeys   []DBBKeyFull
	Keys      []DBBKeyFull
	Offset    uint64
}

func NewKeySlice(Directory string) *KeySlice {
	ks := new(KeySlice)
	ks.Directory=Directory
	return ks
}

func (k *KeySlice) Open(slice int) (err error) {
	k.slice=slice
	
	filename := filepath.Join(k.Directory, fmt.Sprintf("KeyFile_%3d.dat", slice))
	if err = os.MkdirAll(k.Directory,os.ModePerm); err != nil {
		return err
	}

	k.File,err = os.OpenFile(filename,os.O_RDWR,os.ModePerm)
	
	switch {
	case errors.Is(err, os.ErrNotExist):
		if k.File, err = os.Create(filename); err != nil {
			return err
		}
		k.Offset = 8
		var offsetB [8]byte // Write an initial offset of 8 to the new file
		offsetB[7] = 8
		k.File.Write(offsetB[:])
	case err != nil:
		return err
	default:
		if k.File, err = os.OpenFile(filename, os.O_RDWR, os.ModePerm); err != nil {
			return err
		}
		var offsetB [8]byte
		if _, err = k.File.Read(offsetB[:]); err != nil {
			k.File.Close()
			return err
		}
		k.Offset = binary.BigEndian.Uint64(offsetB[:])
		if _, err = k.File.Seek(int64(k.Offset), io.SeekStart); err != nil { // Seek to start of keys.
			k.File.Close()
			return err
		}
		Keys, err := io.ReadAll(k.File)
		if err != nil {
			return err
		}
		k.Keys = make([]DBBKeyFull, len(Keys)/48)
		for i := range k.Keys {
			copy(k.Keys[i].Key[:], Keys[:32])
			k.Keys[i].Offset = binary.BigEndian.Uint64(Keys[32:])
			k.Keys[i].Length = binary.BigEndian.Uint64(Keys[40:])
			Keys = Keys[48:]
		}
		k.File.Seek(int64(k.Offset), io.SeekStart) // Position to add more values
	}
	return nil
}

// Close
// close the KeyFile
func (kf *KeySlice) Close() error {
	
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
func (kf *KeySlice) Put (dbKey DBBKeyFull, value []byte) (err error) {
	dbKey.Offset=kf.Offset
	if _, err = kf.File.Write(value); err != nil {
		return err
	}
	kf.Offset+=uint64(len(value))
	kf.NewKeys=append(kf.NewKeys,dbKey)
	return nil
}


func (kf *KeySlice) Update(bFile *BFile) (err error) {

	keys,err := bFile.GetKeys()
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
