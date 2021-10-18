package managed

import (
	"crypto/sha256"
	"math/rand"
	"sort"
	"testing"

	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/smt/storage/database"
	"github.com/AccumulateNetwork/accumulated/smt/storage/memory"
)

func TestRestart(t *testing.T) {

	dbManager, err := database.NewDBManager("memory", "")
	if err != nil {
		t.Fatalf("could not create database. error: %v", err)
	}

	MarkPower := int64(2)

	// Set up a MM1 that uses a MarkPower of 2
	appID := sha256.Sum256([]byte("root"))

	for i := 0; i < 100; i++ { //                                        Run 100 tests
		MM1, err := NewMerkleManager(dbManager, appID[:], MarkPower) //   Create a MerkleManager
		if err != nil {                                              //   Of course, an error should be reported, but
			t.Errorf("did not create a merkle manager: %v", err) //      won't get one unless something is really sick
		}

		for j := 0; j < 100; j++ { //                                     Add 100 hashes
			if j == i {
				MM1.Manager.EndBatch()                                     // Flush the cache
				MM2, _ := NewMerkleManager(dbManager, appID[:], MarkPower) // Then get the highest state stored
				if !MM1.Equal(MM2) {                                       // MM2 should be the same as MM1
					t.Fatalf("could not restore MM1 in MM2.  index: %d", j) // are going to be messed up.
				}
			}
			for rand.Int()%30 > 0 { //                                       Add 0 or more random hashes
				MM1.AddHash(sha256.Sum256(common.Int64Bytes(rand.Int63()))) // Generate and add one random hash
			}
		}
	}
}

func TestRand(t *testing.T) {
	cnt := uint64(0)

	seed := uint64(0)
	RandInt := func() uint64 {
		cnt++
		seed = seed ^ cnt ^ seed<<7 ^ seed>>3
		return seed<<11 ^ seed>>5
	}

	sums := [1024]uint{}
	total := 0
	for i := 0; i < 1024000000; i++ {
		sums[RandInt()%uint64(len(sums))] += 1
		total++
	}

	sort.Slice(sums[:], func(i, j int) bool { return sums[i] < sums[j] })

	for _, v := range sums[:] {
		println(v)
	}
}

func TestRestartCache(t *testing.T) {

	cnt := uint64(0)

	GetHash := func() [32]byte {
		cnt++
		return sha256.Sum256(common.Uint64Bytes(cnt))
	}

	seed := uint(0)
	RandInt := func() uint {
		cnt++
		seed = seed ^ uint(cnt) ^ seed<<7 ^ seed>>3
		return seed<<11 ^ seed>>5
	}
	dbManager, err := database.NewDBManager("memory", "")
	if err != nil {
		t.Fatalf("could not create database. error: %v", err)
	}

	MarkPower := int64(2)

	// Set up a MM1 that uses a MarkPower of 2
	appID := sha256.Sum256([]byte("root"))

	for i := uint(0); i < 500; i += RandInt() % 10 { //                                        Run 100 tests

		MM1, err := NewMerkleManager(dbManager, appID[:], MarkPower) //   Create a MerkleManager
		if err != nil {                                              //   Of course, an error should be reported, but
			t.Errorf("did not create a merkle manager: %v", err) //      won't get one unless something is really sick
		}

		var cached [][32]byte // We are using this slice to track the hashes that have been written to MM1
		//                       but not yet written to disk by MM1.  When we EndBatch on MM1 we will
		//                       clear this cache.
	TestLoop:
		for j := uint(0); j < 500; j++ { //                                     Add 100 hashes
			for k := uint(0); k < RandInt()%4; k++ { //                                       Add 0 or more random hashes
				h := GetHash()
				cached = append(cached, h)
				MM1.AddHash(h) // Generate and add one random hash
			}

			ended := RandInt()%30 > 0 && len(cached) > 0 // Every so often we are going to write to disk
			if ended {                                   //   so what is cached is going away too.
				MM1.Manager.EndBatch() //  Flush MM1 to disk
				cached = cached[:0]    //  Clear the cache
			}

			if j == i {
				ndm := new(database.Manager) // This simulates opening a brand new database later (MM2)
				ndm.InitWithDB(dbManager.DB.(*memory.DB).Copy())
				MM2, err := NewMerkleManager(ndm, appID[:], MarkPower) // Then get the highest state stored

				if err != nil {
					t.Fatalf("failed to create MM2 properly")
				}

				if MM1.MS.Equal(MM2.MS) && len(cached) > 0 {
					t.Error("MM2 should not be the same as MM1 if some hashes are in the cache still")
				}

				for _, h := range cached {
					MM2.AddHash(h)
				}

				if !MM1.MS.Equal(MM2.MS) { // MM2 should be the same as MM1
					t.Errorf("MM2 isn't tracking MM1 at index: %d %d", i, j) // are going to be messed up.
				}
				break TestLoop
			}
		}

	}
}
