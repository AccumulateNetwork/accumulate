package common

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"
	"time"
)

func TestSliceBytes(t *testing.T) {
	for i := 0; i < 1024; i++ {
		var bytetest []byte
		for j := 0; j < i; j++ {
			bytetest = append(bytetest, byte(rand.Int()))
		}
		counted := SliceBytes(bytetest)
		slice, left := BytesSlice(counted)

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
		counted := SliceBytes(bytetest)
		inputs = append(inputs, counted...)
	}

	for i, v := range tests {
		var slice []byte
		slice, inputs = BytesSlice(inputs)

		if !bytes.Equal(v, slice) {
			t.Errorf(" %d %x %x", i, v, slice)
		}
	}

	if len(inputs) != 0 {
		t.Error("should consume all data")
	}
}

func TestFormatTimeLapse(t *testing.T) {
	var d = time.Duration(time.Hour*3 + time.Minute*4 + time.Second*5)
	str := FormatTimeLapse(d)
	if fmt.Sprint(str) != "    03:04:05 h:m:s  " {
		t.Error("failed to print time as desired")
	}
}
