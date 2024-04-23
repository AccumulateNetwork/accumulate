package multipleDB

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"sync"
)

const (
	ShardBits = 9
	Shards    = 512 // Number of shards in bits
)

// Shard
// Holds the stuff required to access a shard.
type Shard struct {
	File  string     // The file with the BFile
	BFile *BFile     // The BFile
	mutex sync.Mutex // Keeps compression from conflict with access
}

func (s *Shard) Open() {

}

// ShardDB
// Maintains shards of key value pairs to allow reading and writing of
// key value pairs even during compression and eventually multi-thread
// transactions.
type ShardDB struct {
	PermBFile *BFile         // The BFile has the directory and file
	Shards    [Shards]*Shard // List of all the Shards
}

func (s *ShardDB) Create(Directory string) (err error) {
	if s.PermBFile, err = NewBFile(5, Directory, BFilePerm, BFileDN); err != nil {
		return err
	}

	return nil
}

func (s *ShardDB) Close() {
	if s.PermBFile != nil {
		s.PermBFile.Close()
	}
	for _, shard := range s.Shards {
		if shard != nil {
			shard.BFile.Close() // Close everything we have opened
		}
	}
}

func (s *ShardDB) Open(Directory string) (err error) {

	if s.PermBFile, err = OpenBFile(Directory, BFilePerm, BFileDN, 0); err != nil {
		return err
	}

	for i := 0; i < Shards; i++ {
		SDir := filepath.Join(Directory, fmt.Sprintf("shard-%03x", i))
		s.Shards[i] = new(Shard)
		if s.Shards[i].BFile, err = OpenBFile(SDir, BFilePerm, BFileDN, 0); err != nil {

		}
	}

	return nil
}

// PutH
// When the key is the hash (or other function) of the value, where the value will
// never change, then use PutH.  The assumption is that these values, once recorded,
// will not be used in a validator.
func (s *ShardDB) PutH(scratch bool, key [32]byte, value []byte) error {
	k := binary.BigEndian.Uint16(key[:]) >> (16 - ShardBits)
	shard := s.Shards[k]
	if shard == nil {
		shard = new(Shard)
		s.Shards[k] = shard
	}
	return s.PermBFile.Put(key, value)
}

// Put
// Put a key into the database
func (s *ShardDB) Put(key [32]byte, value []byte) {
	k := binary.BigEndian.Uint16(key[:]) >> (16 - ShardBits)
	shard := s.Shards[k]
	if shard == nil {
		shard.Open()
		shard = new(Shard)
		shard.Mod = make(map[[32]byte][]byte)
		s.Shards[k] = shard
	}
	shard.Mod[key] = value
}

// Get
// Get a key from the DB
func (s *ShardDB) Get(key [32]byte) (value []byte) {
	k := binary.BigEndian.Uint16(key[:]) >> (16 - ShardBits)
	shard := s.Shards[k]
	if shard == nil {
		return nil
	}
	v := shard.Mod[key]
	if v == nil {
		v, _ = s.PermBFile.Get(key) // If the err is not nil, v will be, so no need to check err
	}
	return v
}
