package blockchainDB

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

const (
	ShardBits = 9
	Shards    = 512 // Number of shards in bits
)

// ShardDB
// Maintains shards of key value pairs to allow reading and writing of
// key value pairs even during compression and eventually multi-thread
// transactions.
type ShardDB struct {
	PermBFile *BlockList     // The BFile has the directory and file
	BufferCnt int            // Buffer count used for BFiles
	Shards    [Shards]*Shard // List of all the Shards
}

func CreateShardDB(Directory string, Partition, BufferCnt int) (SDB *ShardDB, err error) {
	_, err = os.Stat(Directory)
	if err == nil {
		return nil, fmt.Errorf("cannot create ShardDB; directory %s exists", Directory)
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("error getting status on directory '%s': %v", Directory, err)
	}
	SDB = new(ShardDB)
	SDB.BufferCnt = BufferCnt
	err = os.Mkdir(Directory, os.ModePerm)
	if err != nil {
		return nil, err
	}
	SDB.PermBFile, err = NewBlockList(filepath.Join(Directory, "PermBFile"), Partition, BufferCnt)
	if err != nil {
		return nil, err
	}

	for i := 0; i < Shards; i++ {
		sDir := filepath.Join(Directory, fmt.Sprintf("shard%03d-%03d", Partition, i))
		err = os.Mkdir(sDir, os.ModePerm)
		if err != nil {
			os.RemoveAll(Directory)
			return nil, err
		}
		SDB.Shards[i] = new(Shard)
		SDB.Shards[i].Filename = filepath.Join(sDir, "shard.dat")
		SDB.Shards[i].BufferCnt = BufferCnt
		err = SDB.Shards[i].Open()
		if err != nil {
			os.RemoveAll(Directory)
			return nil, err
		}
	}

	return SDB, nil
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
func (s *ShardDB) Put(key [32]byte, value []byte) error {
	k := binary.BigEndian.Uint16(key[:]) >> (16 - ShardBits)
	shard := s.Shards[k]
	if shard == nil {
		shard = new(Shard)
		shard.Open()
		s.Shards[k] = shard
	}
	return shard.BFile.Put(key, value)
}

// Get
// Get a key from the DB
func (s *ShardDB) Get(key [32]byte) (value []byte) {
	k := binary.BigEndian.Uint16(key[:]) >> (16 - ShardBits)
	shard := s.Shards[k]
	if shard == nil {
		return nil
	}
	v, err := shard.BFile.Get(key)
	if err != nil && v == nil {
		v, _ = s.PermBFile.Get(key) // If the err is not nil, v will be, so no need to check err
	}
	return v
}
