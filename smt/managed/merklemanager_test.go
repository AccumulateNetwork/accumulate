package managed

import (
	"crypto/sha256"
	"math"
	"testing"

	"github.com/AccumulateNetwork/accumulated/smt/storage/database"
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
		MM1.AddHash(sha256.Sum256([]byte{byte(i), byte(i >> 8), byte(i >> 16), byte(i >> 24)}))
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

func TestIndexing(t *testing.T) {
	t.Skip("ignore")
	const testlen = 1024
	const blocklen = 10

	dbManager := new(database.Manager)
	if err := dbManager.Init("memory", ""); err != nil {
		t.Fatal(err)
	}

	MM1, err := NewMerkleManager(dbManager, 2)
	if err != nil {
		t.Fatal("didn't create a Merkle Manager")
	}
	// Fill the Merkle Tree with a few hashes
	hash := sha256.Sum256([]byte("start"))
	for i := 0; i < testlen; i++ {
		MM1.AddHash(hash)
		hash = sha256.Sum256(hash[:])
	}

	hash = sha256.Sum256([]byte("start"))
	for i := int64(0); i < testlen; i++ {
		if (i+1)%blocklen == 0 {
			bi := new(BlockIndex)
			data := MM1.Manager.Key("BlockIndex", "", i/blocklen).Get()
			bi.UnMarshal(data)
			if bi.MainIndex != i {
				t.Fatalf("the MainIndex doesn't match v %d i %d",
					bi.MainIndex, i)
			}
			if bi.BlockIndex != i/blocklen {
				t.Fatalf("the BlockIndex doesn't match v %d i/blocklen %d",
					bi.BlockIndex, i/blocklen)
			}
		}
		if v := MM1.GetIndex(hash[:]); v < 0 {
			t.Fatalf("failed to index hash %d", i)
		} else {
			if v != i {
				t.Fatalf("failed to get the right index.  i %d v %d", i, v)
			}
		}
		hash = sha256.Sum256(hash[:])
	}

	MM2, err := NewMerkleManager(dbManager, 2)
	if err != nil {
		t.Fatal("Did not create a Merkle Manager")
	}
	if MM1.MS.Count != MM2.MS.Count {
		t.Fatal("failed to properly load from a database")
	}

	hash = sha256.Sum256([]byte("start"))
	for i := 0; i < testlen; i++ {
		if v := MM2.GetIndex(hash[:]); v < 0 {
			t.Fatalf("failed to index hash %d", i)
		} else {
			if int(v) != i {
				t.Fatalf("failed to get the right index.  i %d v %d", i, v)
			}
		}
		hash = sha256.Sum256(hash[:])
	}
}

func TestMerkleManager(t *testing.T) {
	t.Skip("ignore")
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
		MM1.AddHash(hash)
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
