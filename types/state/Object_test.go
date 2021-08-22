package state

import (
	"crypto/sha256"
	"testing"
)

func TestStateObject(t *testing.T) {

	so := Object{}

	th := sha256.Sum256([]byte("headerTypeTest"))

	so.Header.Type = th
	so.Header.AdiChainPath = "object/type/path"

	hash := sha256.Sum256([]byte("stateHashTest"))
	so.StateHash = hash[:]

	hash = sha256.Sum256([]byte("prevStateHashTest"))
	so.PrevStateHash = hash[:]

	so.Entry = []byte("This is a fake test entry")

	hash = sha256.Sum256(so.Entry)
	so.EntryHash = hash[:]

	data, err := so.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	so2 := Object{}

	err = so2.Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}

}
