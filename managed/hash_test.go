package managed

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/AccumulateNetwork/SMT/common"
)

func TestSliceBytes(t *testing.T) {
	for i := 0; i < 1024; i++ {
		var bytetest []byte
		for j := 0; j < i; j++ {
			bytetest = append(bytetest, byte(rand.Int()))
		}
		counted := common.SliceBytes(bytetest)
		slice, left := common.BytesSlice(counted)

		if len(left) > 0 || !bytes.Equal(bytetest, slice) {
			t.Errorf(" %d %x %x", len(left), bytetest, slice)
		}
	}

	var tests [][]byte
	var inputs []byte
	for i := 0; i < 1024; i++ {
		var bytetest []byte
		for j := 0; j < i; j++ {
			bytetest = append(bytetest, byte(rand.Int()))
		}
		tests = append(tests, bytetest)
		counted := common.SliceBytes(bytetest)
		inputs = append(inputs, counted...)
	}

	for i, v := range tests {
		var slice []byte
		slice, inputs = common.BytesSlice(inputs)

		if !bytes.Equal(v, slice) {
			t.Errorf(" %d %x %x", i, v, slice)
		}
	}

	if len(inputs) != 0 {
		t.Error("should consume all data")
	}
}
