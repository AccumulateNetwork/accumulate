package managed

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestCopy(t *testing.T) {
	ms1 := new(MerkleState)
	ms1.InitSha256()
	for i := 0; i < 15; i++ {
		hash := Hash(sha256.Sum256([]byte(fmt.Sprintf("%x", i*i*i*i))))
		ms1.AddToMerkleTree(hash)
	}
	ms1.AddToMerkleTree(Hash(sha256.Sum256([]byte{1, 2, 3, 4, 5})))
	ms2 := ms1
	if !ms1.Equal(ms2) {
		t.Error("ms1 should be equal ms2")
	}
	ms2 = ms1.Copy()
	ms3 := ms2.Copy()
	if !ms1.Equal(ms2) || !ms2.Equal(ms3) || !ms1.Equal(ms3) {
		t.Error("ms1 ms2 and ms3 should all be equal")
	}

	ms1.AddToMerkleTree(Hash(sha256.Sum256([]byte{1, 2, 3, 4, 5})))
	if ms1.Equal(ms2) {
		t.Error("ms1 should not equal ms2")
	}
	fmt.Println(ms1.String())
	fmt.Println(ms2.String())
}

func TestMarshal(t *testing.T) {
	MS1 := new(MerkleState)
	MS1.InitSha256()
	MS2 := new(MerkleState)
	MS2.InitSha256()

	data1, err := MS1.Marshal()
	if err != nil {
		t.Fatal("marshal should not fail")
	}
	MS2.UnMarshal(data1)
	data2, err2 := MS2.Marshal()
	if err2 != nil {
		t.Fatal("marshal should not fail")
	}

	if !bytes.Equal(data1, data2) {
		t.Error("Should be the same")
	}

	MS1.AddToMerkleTree(MS1.HashFunction([]byte{1, 2, 3, 4, 5}))

	data1, err = MS1.Marshal()
	if err != nil {
		t.Fatal("marshal should not fail")
	}

	MS2.UnMarshal(data1)
	data2, err = MS2.Marshal()
	if err != nil {
		t.Fatal("marshal should not fail")
	}

	if !MS1.Equal(MS2) {
		t.Error("Should be the same")
	}
	if !bytes.Equal(data1, data2) {
		t.Error("Should be the same")
	}

	for i := 0; i < 10; i++ {
		MS1.AddToMerkleTree(MS1.HashFunction([]byte(fmt.Sprintf("%8d", i))))

		data1, err = MS1.Marshal()
		if err != nil {
			t.Fatal("marshal should not fail")
		}
		MS2.UnMarshal(data1)
		data2, err = MS2.Marshal()
		if err != nil {
			t.Fatal("marshal should not fail")
		}
		if !bytes.Equal(data1, data2) {
			t.Error("Should be the same")
		}
		if !bytes.Equal(data1, data2) {
			t.Error("Should be the same")
		}
	}
}
