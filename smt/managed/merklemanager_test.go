package managed

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
	"testing"

	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/smt/storage/database"
)

func TestMerkleManager_ReadChainHead(t *testing.T) {
	dbManager := new(database.Manager)
	if err := dbManager.Init("memory", ""); err != nil {
		t.Fatal(err)
	}
	MM1, err := NewMerkleManager(dbManager, 2)
	if err != nil {
		t.Fatal("didn't create a Merkle Manager")
	}
	MM1.SetChainID([]byte{1})

	for i := 0; i < 100; i++ {
		MM1.AddHash(Sha256([]byte{byte(i), byte(i >> 8), byte(i >> 16), byte(i >> 24)}))
		err1 := MM1.WriteChainHead([]byte{1})
		if err1 != nil {
			t.Fatalf("didn't write chain head")
		}
		MM1.Manager.EndBatch()
		MM2, err := NewMerkleManager(dbManager, 2)
		MM2.SetChainID([]byte{1})
		if err != nil {
			t.Fatalf("didn't create another Merkle Manager")
		}
		if !MM1.Equal(MM2) {
			t.Fatalf("Couldn't load state")
		}
	}
}

func TestIndexing2(t *testing.T) {
	const testlen = 1024

	dbManager := new(database.Manager)
	if err := dbManager.Init("memory", ""); err != nil {
		t.Fatal(err)
	}

	Chain := sha256.Sum256([]byte("RedWagon/ACME_tokens"))
	BlkIdx := Chain
	BlkIdx[30] += 2

	MM1, err := NewMerkleManager(dbManager, 2)
	if err != nil {
		t.Fatal("didn't create a Merkle Manager")
	}

	if err := MM1.SetChainID(Chain[:]); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < testlen; i++ {
		data := []byte(fmt.Sprintf("data %d", i))
		dataHash := sha256.Sum256(data)
		MM1.AddHash(dataHash[:])
		dataI, e := MM1.Manager.Key(Chain, "ElementIndex", dataHash).Get()
		if e != nil {
			t.Fatalf("error")
		}
		di, _ := common.BytesInt64(dataI)
		if di != int64(i) {
			t.Fatalf("didn't get the right index. got %d expected %d", di, i)
		}
		d, e2 := MM1.Manager.Key(Chain, "Element", i).Get()
		if e2 != nil || !bytes.Equal(d, dataHash[:]) {
			t.Fatalf("didn't get the data back. got %d expected %d", d, data)
		}
	}

}

func TestMerkleManager(t *testing.T) {

	const testLen = 1024

	dbManager := new(database.Manager)
	if err := dbManager.Init("memory", ""); err != nil {
		t.Fatal(err)
	}

	MarkPower := int64(2)
	MarkFreq := int64(math.Pow(2, float64(MarkPower)))
	MarkMask := MarkFreq - 1

	// Set up a MM1 that uses a MarkPower of 2
	MM1, err := NewMerkleManager(dbManager, MarkPower)
	if err != nil {
		t.Fatal("did not create a merkle manager")
	}
	if MarkPower != MM1.MarkPower ||
		MarkFreq != MM1.MarkFreq ||
		MarkMask != MM1.MarkMask {
		t.Fatal("Marks were not correctly computed")
	}

	// Fill the Merkle Tree with a few hashes
	hash := sha256.Sum256([]byte("start"))
	for i := 0; i < testLen; i++ {
		MM1.AddHash(hash[:])
		hash = sha256.Sum256(hash[:])
	}

	if MM1.GetElementCount() != testLen {
		t.Fatal("added elements in merkle tree don't match the number we added")
	}

	dbManager.EndBatch()

	// Sort the Indexing
	for i := int64(0); i < testLen; i++ {
		ms := MM1.GetState(i)
		m := MM1.GetNext(i)
		if (i+1)&MarkMask == 0 {
			if ms == nil {
				t.Fatal("should have a state at Mark point - 1 at ", i)
			}
			if m == nil {
				t.Fatal("should have a next element at Mark point - 1 at ", i)
			}
		} else if i&MarkMask == 0 {
			if ms != nil && i != 0 {
				t.Fatal("should not have a state at Mark point at ", i)
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
