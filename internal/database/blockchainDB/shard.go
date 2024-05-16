package blockchainDB

import (
	"os"
	"sync"
)

// Shard
// Holds the stuff required to access a shard.
type Shard struct {
	BufferCnt int                 // Number of buffers to use
	Filename  string              // The file with the BFile
	BFile     *BFile              // The BFile
	Cache     map[[32]byte][]byte // Cache of what is being written to the cache
	Mutex     sync.Mutex          // Keeps compression from conflict with access
}

// NewShard
// Create and open a new Shard
func NewShard(BufferCnt int, Filename string) (shard *Shard, err error) {
	shard = new(Shard)
	shard.BufferCnt = BufferCnt
	shard.Filename = Filename
	shard.Cache = make(map[[32]byte][]byte)
	if shard.BFile, err = NewBFile(Filename, BufferCnt); err != nil {
		return nil, err
	}
	return shard, err
}

// Close
// Clean up and close the Shard
func (s *Shard) Close() {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	s.Cache = nil
	s.BFile.Close()
	s.BFile = nil
}

// Open
// Open an existing Shard.  If the BFile does not exist, create it
func (s *Shard) Open() (err error) {
	if s.BFile != nil {
		return
	}
	if s.BFile, err = OpenBFile(s.BufferCnt, s.Filename); err != nil {
		if os.IsNotExist(err) {
			s.BFile, err = NewBFile(s.Filename, s.BufferCnt)
		}
		return err
	}
	return nil
}

// Put
// Put a key value pair into the shard
func (s *Shard) Put (key [32]byte, value []byte) error {
	s.Cache[key]=value
	return s.BFile.Put(key,value)
}

// Get
// Get the value for the key out of the shard
func (s *Shard) Get (key [32]byte) ( value []byte, err error) {
	if value, ok := s.Cache[key]; ok {
		return value, nil
	}
	return s.BFile.Get(key)
}