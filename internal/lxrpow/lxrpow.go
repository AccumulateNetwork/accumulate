// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package lxrpow

import (
	"encoding/binary"
	"time"
)

// LXRHash uses a combination of shifts and exclusive or's to combine inputs and states to
// create a hash with strong avalanche properties. This means that if any one bit is changed
// in the inputs, on average half of the bits of the output change. This quality makes an
// algorithmic attack on mining very, very unlikely.
//
// First, the hash and nonce are "mixed" such that a reasonable avalanche property is obtained.
//
// Secondly, the resulting bytes are mapped through a byteMap where random accesses have
// an even chance of producing a value between 0-255. Cycling through lookups from random bytes in
// the byteMap space produces a resulting value to be graded. In the example code provided, the
// highest unsigned value is sought (representing higher difficulty).
//
// The Shift XoR Random Number Generator (RNG) was invented by George Marsaglia and published
// in 2003.

// LxrPow represents the LXR proof of work algorithm
type LxrPow struct {
	Loops   uint64 // The number of loops translating the ByteMap
	ByteMap []byte // Integer Offsets
	MapBits uint64 // Number of bits for the MapSize (2^MapBits = MapSize)
	MapSize uint64 // Size of the translation table (must be a power of 2)
	Passes  uint64 // Passes to generate the rand table
}

// NewLxrPow creates a new instance of the LxrPow work function
//
// Parameters:
// - Loops: the number of passes through the hash made through the hash itself. More loops will slow down the hash function.
// - Bits: number of bits used to create the ByteMap. 30 bits creates a 1 GB ByteMap
// - Passes: Number of shuffles used to randomize the ByteMap. 6 seems sufficient
//
// Any change to Loops, Bits, or Passes will map the PoW to a completely different space.
func NewLxrPow(Loops, Bits, Passes uint64) *LxrPow {
	lx := new(LxrPow)
	lx.Init(Loops, Bits, Passes)
	return lx
}

// Init initializes the LxrPow instance with the given parameters
func (lx *LxrPow) Init(Loops, Bits, Passes uint64) {
	lx.Loops = Loops
	lx.MapBits = Bits
	lx.MapSize = uint64(1) << Bits
	lx.Passes = Passes
	lx.ByteMap = make([]byte, lx.MapSize)
	lx.GenerateByteMap()
}

// GenerateByteMap creates the byte map used for the LXR hash
func (lx *LxrPow) GenerateByteMap() {
	// Initialize the ByteMap with sequential values
	for i := range lx.ByteMap {
		lx.ByteMap[i] = byte(i)
	}

	// Shuffle the ByteMap
	for i := uint64(0); i < lx.Passes; i++ {
		// Use a different seed for each pass
		seed := uint64(0xFAB3C4E9D2A18567) + i
		for j := uint64(0); j < lx.MapSize; j++ {
			// Generate a random index
			seed = seed<<19 ^ seed>>7 ^ j
			k := seed & (lx.MapSize - 1)
			// Swap the bytes
			lx.ByteMap[j], lx.ByteMap[k] = lx.ByteMap[k], lx.ByteMap[j]
		}
	}
}

// LxrPoW returns a uint64 value indicating the proof of work of a hash
// The bigger uint64, the more PoW it represents. The first byte is the
// number of leading bytes of FF, followed by the leading "non FF" bytes of the pow.
func (lx *LxrPow) LxrPoW(hash []byte, nonce uint64) uint64 {
	_, state := lx.LxrPoWHash(hash, nonce)
	return state
}

// LxrPoWHash returns both the hash and the proof of work value
func (lx *LxrPow) LxrPoWHash(hash []byte, nonce uint64) ([]byte, uint64) {
	mask := lx.MapSize - 1

	LHash, state := lx.mix(hash, nonce)

	// Make the specified "loops" through the LHash
	for i := uint64(0); i < lx.Loops; i++ {
		for j := range LHash {
			b := uint64(lx.ByteMap[state&mask]) // Hit the ByteMap
			v := uint64(LHash[j])               // Get the value from LHash as a uint64
			state ^= state<<17 ^ b<<31          // A modified XorShift RNG
			state ^= state>>7 ^ v<<23           // Fold in a random byte from
			state ^= state << 5                 // the byteMap and the byte
			LHash[j] = byte(state)              // from the hash.
		}
		time.Sleep(0) // Allow other goroutines to run
	}
	_, state = lx.mix(LHash[:], state)
	return LHash[:], state // Return the pow of the translated hash
}

// mix takes the nonce and the hash and mixes the bits of both into a byte array
// such that changing one bit pretty much changes the odds of changing any bit
// by 50% ish. In other words, mix stands in for a cryptographic hash, speeding
// up the time it takes to get to translating through the ByteMap.
func (lx *LxrPow) mix(hash []byte, nonce uint64) ([32]byte, uint64) {
	if len(hash) != 32 {
		panic("must provide a 32 byte hash")
	}
	state := uint64(0xb32c25d16e362f32) // Set the state to an arbitrary unsigned long with bits
	nonce ^= nonce<<19 ^ state          // set and not set in each byte. Run the nonce through a
	nonce ^= nonce >> 1                 // Xorshift RNG
	nonce ^= nonce << 3

	var array [5]uint64      // Array of uint64, the nonce and 4 longs holding hash
	array[0] = nonce         // This array along with the state is the sponge for the LXRHash
	for i := 1; i < 5; i++ { // Walk through the last 4 elements of array
		array[i] = binary.BigEndian.Uint64(hash[(i-1)<<3:]) // Get 8 bytes of the hash as array entry
		state ^= state<<7 ^ array[i]                        // shift state combine the whole hash into
		state ^= state >> 3                                 // the state while randomizing the state
		state ^= state << 5                                 // through a Xorshift RNG.
	}

	// Mix what is effectively sponge of 5 elements and a state
	for i, a := range array {
		left1 := (a & 0x1F) | 1                // Use the element to pick 3 random shifts for the Xorshift RNG
		right := ((a & 0xF0) >> 5) | 1         // Note that the right is smaller than the left
		left2 := ((a&0x1F0000)>>17)<<1 ^ left1 // And xor-ing left1 with left2 ensures they are different
		state ^= state<<left1 ^ a<<5           // Randomize the state and array element
		state ^= state >> right                // using the random shifts
		state ^= state << left2                //
		array[i] ^= state                      // apply to the array
	}

	// Extract the 32 byte hash and mix the state a bit more
	var newHash [32]byte
	for i, a := range array[:4] {
		a ^= state
		binary.BigEndian.PutUint64(newHash[i*8:], a)
		state ^= state<<27 ^ uint64(i)<<19 // Mix the state a bit more, fold in the counter
		state ^= state >> 11               //
		state ^= state << 13               //
	}

	return newHash, state
}

// DefaultLxrPow returns a default LxrPow instance with reasonable parameters
func DefaultLxrPow() *LxrPow {
	return NewLxrPow(5, 30, 6) // 5 loops, 1GB byte map (2^30), 6 passes
}
