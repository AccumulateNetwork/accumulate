package managed

import (
	"crypto/sha256"
	"fmt"
)

type AllowedMap struct {
	hashes map[[32]byte]int
	lock   bool
}

// Chk
// Return the same hash passed to Check so Check can be called
// inline anywhere a Hash is used.  We hash our input so anything
// can be put in the map, not just hashes.
func (a AllowedMap) Chk(hash []byte) []byte {
	var key = sha256.Sum256(hash)
	if !a.lock {
		a.Adh(hash)
	}
	if _, ok := a.hashes[key]; !ok {
		panic(fmt.Sprintf("not a valid hash: %x", hash))
	}
	return hash
}

// Adh
// Add a hash to AllowedMap so we can check against it latter.
// Add a hash to a map so it can be allowed to be used latter.  Passes
// back the same hash so it can be used inline.
func (a *AllowedMap) Adh(hash []byte) []byte {
	if a.hashes == nil {
		a.hashes = make(map[[32]byte]int)
	}
	if !a.lock {
		var key = sha256.Sum256(hash)
		a.hashes[key] = a.hashes[key] + 1
	}
	return hash
}

// SetLock
// Lock the AllowedMap to ignore more additions to the set
func (a *AllowedMap) SetLock(bool) {
	a.lock = true
}
