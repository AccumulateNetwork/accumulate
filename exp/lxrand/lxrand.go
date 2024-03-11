// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package lxrand

import "encoding/binary"

type Sequence struct {
	cnt   uint64
	state uint64
	seed  [32]byte
}

// Randomize the state and seed of the LXRandom generator
func (r *Sequence) spin() {
	if r.cnt == 0 { //                  First time this instance has been called, so the init
		r.state = 0x123456789ABCDEF0 // Give the state a value
	}

	for i := 0; i < 32; i += 8 {
		r.cnt++
		r.state = r.cnt ^ r.state<<17 ^ r.state>>7 ^ binary.BigEndian.Uint64(r.seed[i:]) // Shake the state
		binary.BigEndian.PutUint64(r.seed[i:], r.state<<23^r.state>>17)
		r.seed[i] ^= byte(r.state) ^ r.seed[i] // Shake the seed
	}
}

// SetState
// Modifies the state using the given seed and state. Any call to SetState will
// create a different sequence of random values.
func (r *Sequence) SetRandomSequence(state uint64, seed [32]byte) {
	r.state = r.state<<19 ^ r.state>>3 ^ state
	r.seed = seed
	r.spin() // a zero seed requires a few spins to randomize the state
	r.spin() // so to be safe, spin is called 3 times to shake up the state
	r.spin()
}

// Hash
// Return a 32 byte array of random bytes
func (r *Sequence) Hash() [32]byte {
	r.spin()
	return r.seed
}

// Int
// Return a random int
func (r *Sequence) Uint() int {
	r.spin()
	i := int(r.state)
	if i < 0 {
		return -i
	}
	return i
}

// Byte
// Return a random byte
func (r *Sequence) Byte() byte {
	r.spin()
	return r.seed[0]
}

// Slice
// Return a slice of the specified length of random bytes
func (r *Sequence) Slice(length int) (slice []byte) {
	slice = make([]byte, length)
	for i := 0; i < length; i += 32 {
		r.spin()
		copy(slice[i%32:], r.seed[:])
	}
	return slice
}
