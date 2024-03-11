// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package core

import "crypto/sha256"

type Hash []byte

func (h Hash) Copy() Hash {
	g := make([]byte, len(h))
	copy(g, h)
	return g
}

func (h Hash) Combine(g Hash) Hash {
	digest := sha256.New()
	_, _ = digest.Write(h)
	_, _ = digest.Write(g)
	return digest.Sum(nil)
}

func (h Hash) As32() [32]byte {
	return *(*[32]byte)(h)
}

func (h Hash) Bytes() []byte {
	return h
}
