package managed

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

func TestRestart(t *testing.T) {
	store := memory.NewDB()
	storeTx := store.Begin(true)

	MarkPower := int64(2)

	for i := 0; i < 20; i++ { //                                      Run 100 tests
		MM1, err := NewMerkleManager(storeTx, MarkPower) //           Create a MerkleManager
		if err != nil {                                  //           Of course, an error should be reported, but
			t.Errorf("did not create a merkle manager: %v", err) //      won't get one unless something is really sick
		}

		for j := 0; j < 100; j++ { //                                   Add 100 hashes
			if j == i {
				MM2, _ := NewMerkleManager(storeTx, MarkPower) // Then get the highest state stored
				if !MM1.Equal(MM2) {                           // MM2 should be the same as MM1
					t.Fatalf("could not restore MM1 in MM2.  index: %d", j) // are going to be messed up.
				}
			}
			for rand.Int()%3 > 0 { //                                       Add 0 or more random hashes
				MM1.AddHash(Sha256(common.Int64Bytes(rand.Int63())), false) // Generate and add one random hash
			}
		}
	}
}

func TestRestartCache(t *testing.T) {
	rand.Seed(12344)
	var rh common.RandHash
	store := memory.NewDB()
	storeTx := store.Begin(true)

	MarkPower := int64(2)

	for i := uint(0); i < 50; i += uint(rand.Int()) % 10 { //

		MM1, err := NewMerkleManager(storeTx, MarkPower) //       Create a MerkleManager
		if err != nil {                                  //       Of course, an error should be reported, but
			t.Errorf("did not create a merkle manager: %v", err) //  won't get one unless something is really sick
		}

		var cached [][]byte // Using this slice to track the hashes that have been written to MM1
		//                       but not yet written to disk by MM1.  Calling EndBatch on MM1 will need to
		//                       clear this cache.
	TestLoop:
		for j := uint(0); j < 50; j++ { //             Add 100 hashes
			for k := uint(0); k < uint(rand.Int())%4; k++ { //  Add 0 or more random hashes
				h := rh.Next()
				cached = append(cached, h)
				MM1.AddHash(h, false) // Generate and add one random hash
			}

			ended := rand.Int()%30 > 0 && len(cached) > 0 // Every so often we are going to write to disk
			if ended {                                    //   so what is cached is going away too.
				require.NoError(t, storeTx.Commit())
				storeTx = store.Begin(true)
				cached = cached[:0] //  Clear the cache
				MM1, err = NewMerkleManager(storeTx, MarkPower)
				require.NoError(t, err)
			}

			if j == i {
				ndb := store.Copy()                                      // This simulates opening a new database later (MM2)
				MM2, err := NewMerkleManager(ndb.Begin(true), MarkPower) // Then get the highest state stored

				if err != nil {
					t.Fatalf("failed to create MM2 properly")
				}

				if MM1.MS.Equal(MM2.MS) && len(cached) > 0 {
					t.Error("MM2 should not be the same as MM1 if some hashes are in the cache still")
				}

				for _, h := range cached {
					MM2.AddHash(h, false)
				}

				if !MM1.MS.Equal(MM2.MS) { // MM2 should be the same as MM1
					MM1.MS.Equal(MM2.MS)
					t.Errorf("MM2 isn't tracking MM1 at index: %d %d", i, j) // are going to be messed up.
				}
				break TestLoop
			}
		}

	}
}
