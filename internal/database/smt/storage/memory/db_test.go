// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package memory

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

func GetKey(key []byte) (dbKey [32]byte) {
	dbKey = sha256.Sum256(key)
	return dbKey
}

func TestDatabase(t *testing.T) {
	db := New(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	for i := 0; i < 10000; i++ {
		err := batch.Put(GetKey([]byte(fmt.Sprintf("answer %d", i))), []byte(fmt.Sprintf("%x this much data ", i)))
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 10000; i++ {

		val, e := batch.Get(GetKey([]byte(fmt.Sprintf("answer %d", i))))
		if e != nil {
			t.Fatalf("no value found for %d", i)
		}

		if string(val) != fmt.Sprintf("%x this much data ", i) {
			t.Error("Did not read data properly")
		}
	}
}
