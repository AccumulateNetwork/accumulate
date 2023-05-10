// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestHash(t *testing.T) {
	var h Hash
	var data = []byte("abc")
	h = doSha(data)
	h2 := h.Copy()
	if !bytes.Equal(h[:], h2[:]) {
		t.Error("copy failed")
	}
	h = h.Combine(h2)
	h3 := sha256.Sum256(data)
	result := sha256.Sum256(append(h3[:], h3[:]...))
	if !bytes.Equal(h[:], result[:]) {
		t.Error("combine failed")
	}
}
