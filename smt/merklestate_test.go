package smt

import (
	"bytes"
	"fmt"
	"testing"
)

func TestMarshal(t *testing.T) {
	MS1 := new(MerkleState)
	MS1.InitSha256()
	MS2 := new(MerkleState)
	MS2.InitSha256()

	data1 := MS1.Marshal()
	MS2.UnMarshal(data1)
	data2 := MS2.Marshal()
	if !bytes.Equal(data1, data2) {
		t.Error("Should be the same")
	}

	MS1.AddToMerkleTree(MS1.HashFunction([]byte{1, 2, 3, 4, 5}))

	data1 = MS1.Marshal()
	MS2.UnMarshal(data1)
	data2 = MS2.Marshal()
	if !MS1.Equal(*MS2) {
		t.Error("Should be the same")
	}
	if !bytes.Equal(data1, data2) {
		t.Error("Should be the same")
	}

	for i := 0; i < 10; i++ {
		MS1.AddToMerkleTree(MS1.HashFunction([]byte(fmt.Sprintf("%8d", i))))

		data1 = MS1.Marshal()
		MS2.UnMarshal(data1)
		data2 = MS2.Marshal()
		if !bytes.Equal(data1, data2) {
			t.Error("Should be the same")
		}
		if !bytes.Equal(data1, data2) {
			t.Error("Should be the same")
		}
	}

	for i := 0; i < 10; i++ {
		data1 = MS1.Marshal()
		MS2.UnMarshal(data1)
		data1a, hash1 := MS1.EndBlock()
		hash1a := MS1.HashFunction(data1a)
		if !bytes.Equal(data1, data1a) ||
			!bytes.Equal(hash1[:], hash1a[:]) ||
			!bytes.Equal(data1a, MS2.Marshal()) {
			t.Error("Should be the same")
		}
		for j := 0; j < 0; j++ {
			someData := MS1.HashFunction(hash1a[:])
			MS1.AddToMerkleTree(someData)
		}
		MS2.EndBlock()
		for _, v := range MS1.HashList {
			MS2.AddToMerkleTree(v)
		}
		if !bytes.Equal(MS1.Marshal(), MS2.Marshal()) {
			t.Error("should be the same")
		}
	}
}
