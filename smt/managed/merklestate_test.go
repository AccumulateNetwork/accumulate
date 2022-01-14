package managed

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/stretchr/testify/require"
)

func TestMerkleState_Equal(t *testing.T) {
	var rh, rh1 common.RandHash
	ms1 := new(MerkleState)
	ms1.InitSha256()
	ms2 := new(MerkleState)
	ms2.InitSha256()
	for i := 0; i < 100; i++ {
		require.Truef(t, ms1.Equal(ms2), "Should be equal")
		ms1.Pending = append(ms1.Pending, nil)
		require.Truef(t, ms1.Equal(ms2), "Should be equal")
		ms2.Pending = append(ms2.Pending, nil, nil)
		require.Truef(t, ms1.Equal(ms2), "Should be equal")

		ms1.AddToMerkleTree(rh.Next())

		require.Falsef(t, ms1.Equal(ms2), "Should be equal")
		ms1.Pending = append(ms1.Pending, nil)
		require.Falsef(t, ms1.Equal(ms2), "Should be equal")
		ms2.Pending = append(ms2.Pending, nil, nil)
		require.Falsef(t, ms1.Equal(ms2), "Should be equal")

		ms2.AddToMerkleTree(rh1.Next())
		require.Truef(t, ms1.Equal(ms2), "Should be equal")
		ms1.Pending = append(ms1.Pending, nil)
		require.Truef(t, ms1.Equal(ms2), "Should be equal")
		ms2.Pending = append(ms2.Pending, nil, nil)
		require.Truef(t, ms1.Equal(ms2), "Should be equal")

	}
}

func TestCopy(t *testing.T) {
	ms1 := new(MerkleState)
	ms1.InitSha256()
	for i := 0; i < 15; i++ {
		hash := Sha256([]byte(fmt.Sprintf("%x", i*i*i*i)))
		ms1.AddToMerkleTree(hash)
	}
	ms1.AddToMerkleTree(Sha256([]byte{1, 2, 3, 4, 5}))
	ms2 := ms1
	if !ms1.Equal(ms2) {
		t.Error("ms1 should be equal ms2")
	}
	ms2 = ms1.Copy()
	ms3 := ms2.Copy()
	if !ms1.Equal(ms2) || !ms2.Equal(ms3) || !ms1.Equal(ms3) {
		t.Error("ms1 ms2 and ms3 should all be equal")
	}

	ms1.AddToMerkleTree(Sha256([]byte{1, 2, 3, 4, 5}))
	if ms1.Equal(ms2) {
		t.Error("ms1 should not equal ms2")
	}
	//fmt.Println(ms1.String())
	//fmt.Println(ms2.String())
}

func TestUnmarshalMemorySafety(t *testing.T) {
	// Create a Merkle state and add entries
	MS1 := new(MerkleState)
	MS1.InitSha256()
	for i := 0; i < 10; i++ {
		MS1.AddToMerkleTree(MS1.HashFunction([]byte(fmt.Sprintf("%8d", i))))
	}

	// Marshal and unmarshal into a new state
	data, err := MS1.Marshal()
	require.NoError(t, err)
	MS2 := new(MerkleState)
	require.NoError(t, MS2.UnMarshal(data))

	// Overwrite the data array with garbage
	rand.Read(data)

	// Ensure that MS2 did not change
	MS1.HashFunction = nil
	require.True(t, MS1.Equal(MS2), "States do not match")
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
