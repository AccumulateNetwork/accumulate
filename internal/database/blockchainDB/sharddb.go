package blockchainDB

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// ShardDB
// Maintains shards of key value pairs to allow reading and writing of
// key value pairs even during compression and eventually multi-thread
// transactions.
type ShardDB struct {
	PermBFile *BlockList // The BFile has the directory and file
	BufferCnt int        // Buffer count used for BFiles
	Shards    []*Shard   // List of all the Shards
}

// NewShardDB
// Creates a ShardDB directory and structures.  Will overwrite an existing
// file or directory if it exists.
func NewShardDB(Directory string, Partition, ShardCnt, BufferCnt int) (SDB *ShardDB, err error) {
	os.RemoveAll(Directory)

	SDB = new(ShardDB)
	SDB.BufferCnt = BufferCnt
	SDB.Shards = make([]*Shard, ShardCnt)
	err = os.Mkdir(Directory, os.ModePerm)
	if err != nil {
		return nil, err
	}
	f, e := os.Create(filepath.Join(Directory, "state.dat"))
	if e != nil {
		return nil, err
	}
	defer f.Close()

	f.Write([]byte{byte(ShardCnt >> 8), byte(ShardCnt)}) // Write big endian 16 bit shard cnt

	SDB.PermBFile, err = NewBlockList(filepath.Join(Directory, "PermBFile.dat"), Partition, BufferCnt)
	if err != nil {
		return nil, err
	}

	for i := 0; i < len(SDB.Shards); i++ {
		sDir := filepath.Join(Directory, fmt.Sprintf("shard%03d-%03d", Partition, i))
		err = os.Mkdir(sDir, os.ModePerm)
		if err != nil {
			os.RemoveAll(Directory)
			return nil, err
		}
		SDB.Shards[i], err = NewShard(BufferCnt, filepath.Join(sDir, "shard.dat"))
		if err != nil {
			os.RemoveAll(Directory)
			return nil, err
		}
	}

	return SDB, nil
}

// OpenShardDB
// Opens an existing ShardDB.
func OpenShardDB(Directory string, Partition, BufferCnt int) (SDB *ShardDB, err error) {

	// Get the ShardCnt from the ShardDB state.dat file.
	var ShardCntBuff [2]byte
	f, e1 := os.Open(filepath.Join(Directory, "state.dat"))
	defer f.Close()
	_, e2 := f.Read(ShardCntBuff[:])
	switch {
	case e1 != nil:
		return nil, e1
	case e2 != nil:
		return nil, e2
	}
	ShardCnt := int(binary.BigEndian.Uint16(ShardCntBuff[:]))
	_ = ShardCnt
	// Open the shards
	SDB = new(ShardDB)
	//SDB.PermBFile, err = Open(BufferCnt, filepath.Join(Directory,"PermBFile.dat"))
	//SDB.Shards = make([]*Shards,ShardCnt)
	for i := 0; i < len(SDB.Shards); i++ {
		sDir := filepath.Join(Directory, fmt.Sprintf("shard%03d-%03d", Partition, i))
		if SDB.Shards[i], err = OpenShard(BufferCnt, filepath.Join(Directory, sDir, "shard.dat")); err != nil {
			return nil, err
		}
	}
	return nil, nil
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

// GetShard
// Get the shard responsible for a given key
func (s *ShardDB) GetShard(key [32]byte) *Shard {
	v := int(binary.BigEndian.Uint16(key[:2]))
	i := v % len(s.Shards)
	return s.Shards[i]
}

func (s *ShardDB) PutH(scratch bool, key [32]byte, value []byte) error {
	return s.PermBFile.Put(key, value)
}

// Put
// Put a key into the database
func (s *ShardDB) Put(key [32]byte, value []byte) error {
	shard := s.GetShard(key)
	return shard.BFile.Put(key, value)
}

// Get
// Get a key from the DB
func (s *ShardDB) Get(key [32]byte) (value []byte) {
	shard := s.GetShard(key)
	v, err := shard.BFile.Get(key)
	if err != nil && v == nil {
		v, _ = s.PermBFile.Get(key) // If the err is not nil, v will be, so no need to check err
	}
	return v
}
