package smt_test

import (
	"crypto/sha256"
	"math"
	"testing"

	. "github.com/AccumulateNetwork/SMT/smt"
	"github.com/AccumulateNetwork/SMT/storage/database"
)

func TestIndexing(t *testing.T) {

	const testlen = 1024

	dbManager := new(database.Manager)
	if err := dbManager.Init("memory", ""); err != nil {
		t.Fatal(err)
	}

	MerkleManager := new(MerkleManager)
	MerkleManager.Init(dbManager, 2)

	// Fill the Merkle Tree with a few hashes
	hash := sha256.Sum256([]byte("start"))
	for i := 0; i < testlen; i++ {
		MerkleManager.AddHash(hash)
		hash = sha256.Sum256(hash[:])
	}

	MerkleManager.DBManager.EndBatch()
	hash = sha256.Sum256([]byte("start"))
	for i := 0; i < testlen; i++ {
		if v := MerkleManager.GetIndex(hash[:]); v < 0 {
			t.Fatalf("failed to index hash %d", i)
		}
		hash = sha256.Sum256(hash[:])
	}
}

func TestMerkleManager(t *testing.T) {

	const testlen = 1024

	dbManager := new(database.Manager)
	if err := dbManager.Init("memory", ""); err != nil {
		t.Fatal(err)
	}

	MarkPower := int64(2)
	MarkFreq := int64(math.Pow(2, float64(MarkPower)))
	MarkMask := MarkFreq - 1

	// Set up a MerkleManager that uses a MarkPower of 2
	MerkleManager := new(MerkleManager)
	MerkleManager.Init(dbManager, MarkPower)

	if MarkPower != MerkleManager.MarkPower ||
		MarkFreq != MerkleManager.MarkFreq ||
		MarkMask != MerkleManager.MarkMask {
		t.Fatal("Marks were not correctly computed")
	}

	// Fill the Merkle Tree with a few hashes
	hash := sha256.Sum256([]byte("start"))
	for i := 0; i < testlen; i++ {
		MerkleManager.AddHash(hash)
		hash = sha256.Sum256(hash[:])
	}

	if MerkleManager.GetElementCount() != testlen {
		t.Fatal("added elements in merkle tree don't match the number we added")
	}

	dbManager.EndBatch()

	// Check the Indexing
	for i := int64(0); i < testlen; i++ {
		ms := MerkleManager.GetState(i)
		m := MerkleManager.GetNext(i)
		if (i+1)&MarkMask == 0 {
			if ms == nil {
				t.Fatal("should have a state at Mark point - 1 at ", i)
			}
			if m == nil {
				t.Fatal("should have a next element at Mark point - 1 at ", i)
			}
		} else if i&MarkMask == 0 {
			if ms == nil {
				t.Fatal("should have a state at Mark point at ", i)
			}
			if m != nil {
				t.Fatal("should not have a next element at Mark point at ", i)
			}
		} else {
			if ms != nil {
				t.Fatal("should not have a state outside Mark points at ", i)
			}
			if m != nil {
				t.Fatal("should not have a next element outside Mark points at ", i)
			}
		}

	}
}
