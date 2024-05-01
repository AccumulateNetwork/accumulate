package blockchainDB

import (
	"encoding/binary"

	"github.com/minio/sha256-simd"
)

type FastRandom struct {
	sponge [256]uint64
	seed   [32]byte
	index  uint64
	state  uint64
}

func (f *FastRandom) Step() {
	f.state ^= f.sponge[f.index] 
	f.state ^= f.index
	f.state ^= f.state << 11
	f.state ^= f.state >> 15
	f.state ^= f.state << 3
	f.sponge[f.index] ^= f.state
	f.seed[f.index&0x1F] ^= byte(f.state)
	f.index+=7 // Avoid the fact that 32 is a factor of 256
	f.index &= 0xFF
}

func NewFastRandom(seed [32]byte) *FastRandom {
	f := new(FastRandom)
	f.seed = seed
	for i := range f.sponge { // Fill the sponge with parts of hashes of hashes
		f.seed = sha256.Sum256(f.seed[:])
		f.sponge[i] = binary.BigEndian.Uint64(f.seed[:])
	}
	for i:= 0;i<512;i++{
		f.Step()
	}
	return f
}

func (f *FastRandom) Uint64() uint64 {
	f.Step()
	return f.state
}

// Return an int
func (f *FastRandom) UintN(N uint) uint {
	f.Step()
	return uint(f.state) % N
}

func (f *FastRandom) NextHash() (hash [32]byte) {
	for i := 0; i < 32; i ++ {
		hash[i]=byte(f.state)
		f.Step()
	}
	return f.seed
}

// RandBuff
// Always returns a buffer of random bytes 
// If max > 100 MB, max is set to 100 MB
// If max < 1, max is set to 1
// If min > max, min is set to max
func (f *FastRandom) RandBuff(min uint, max uint) []byte {
	if max >= 1024*1024*100 {
		max = 1024*1024*100
	}
	if max <= 0 {
		max = 1
	}
	if min >= max {
		min = max
	}
	byteCount := max;
	if min != max {
		byteCount = f.UintN(max-min) + min
	}
	buff := make([]byte, byteCount)
	count8 := byteCount / 8
	for i := uint(0); i < count8*8; i += 8 {
		binary.BigEndian.PutUint64(buff[i:], f.Uint64())
	}
	for i := count8 * 8; i < byteCount; i++ {
		buff[i] = byte(f.sponge[f.index&15])
	}
	return buff
}
