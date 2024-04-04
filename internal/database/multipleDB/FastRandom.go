package multipleDB

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

func NewFastRandom(seed [32]byte) *FastRandom {
	f := new(FastRandom)
	f.seed = seed
	for i := range f.sponge { // Fill the sponge with parts of hashes of hashes
		f.seed = sha256.Sum256(f.seed[:])
		f.sponge[i] = binary.BigEndian.Uint64(f.seed[:])
	}
	f.index = f.sponge[0] // start the index with first hash
	f.state = f.sponge[1] // start the state with the second
	f.Uint64()            // Mixup up by tossing the first rand value
	return f
}

func (f *FastRandom) Uint64() uint64 {
	f.state ^= f.sponge[f.index&15]
	f.state ^= f.state << 11
	f.state ^= f.state >> 15
	f.state ^= f.state << 3
	f.index ^= f.state
	f.sponge[f.index&15] ^= f.state
	return f.state
}

// Return an int
func (f *FastRandom) UintN(N uint) uint {
	f.state ^= f.sponge[f.index&15]
	f.state ^= f.state << 11
	f.state ^= f.state >> 15
	f.state ^= f.state << 3
	f.index ^= f.state
	f.sponge[f.index&15] ^= f.state
	return uint(f.state) % N
}

func (f *FastRandom) NextHash() (hash [32]byte) {
	for i := 0; i < 32; i += 8 {
		binary.BigEndian.PutUint64(hash[i:], f.Uint64())
	}
	return hash
}

func (f *FastRandom) RandBuff(min uint, max uint) []byte {
	byteCount := f.UintN(max-min) + min
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
