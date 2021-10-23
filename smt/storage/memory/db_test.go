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

	db := new(DB)
	_ = db.InitDB("test")

	for i := 0; i < 10000; i++ {
		err := db.Put(GetKey([]byte(fmt.Sprintf("answer %d", i))), []byte(fmt.Sprintf("%x this much data ", i)))
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 10000; i++ {
		var val []byte
		val = db.Get(GetKey([]byte(fmt.Sprintf("answer %d", i))))
		if val == nil {
			t.Fatalf("no value found for %d", i)
		}

		if string(val) != fmt.Sprintf("%x this much data ", i) {
			t.Error("Did not read data properly")
		}
	}
}
