package pmt_test

import (
	"crypto/sha256"
	"testing"

	. "gitlab.com/accumulatenetwork/accumulate/smt/pmt"
)

func TestValue_Equal(t *testing.T) {
	value := new(Value)
	value.Key = sha256.Sum256([]byte{1})
	value.Hash = sha256.Sum256([]byte{2})

	value2 := new(Value)
	value2.Key = sha256.Sum256([]byte{1})
	value2.Hash = sha256.Sum256([]byte{2})

	if !value.Equal(value2) {
		t.Error("value should be the same")
	}
}
