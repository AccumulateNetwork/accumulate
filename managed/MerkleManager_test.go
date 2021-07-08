package managed

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/AccumulateNetwork/SMT/storage/database"
)

func TestAddSalt(t *testing.T) {
	Salt := sha256.Sum256([]byte{1})
	Salt2 := add2Salt(Salt[:], 1)
	if bytes.Equal(Salt[:], Salt2) {
		t.Errorf("These should not be equal \n%x \n%x", Salt, Salt2)
	}
}

func TestIndexing(t *testing.T) {

	const testlen = 1024
	const blocklen = 10

	dbManager := new(database.Manager)
	if err := dbManager.Init("memory", ""); err != nil {
		t.Fatal(err)
	}

	salt := sha256.Sum256([]byte("root"))
	MM1 := NewMerkleManager(dbManager, salt[:], 2)

	// Fill the Merkle Tree with a few hashes
	hash := sha256.Sum256([]byte("start"))
	for i := 0; i < testlen; i++ {
		MM1.AddHash(hash)
		hash = sha256.Sum256(hash[:])
		if (i+1)%blocklen == 0 {
			MM1.SetBlockIndex(int64(i) / blocklen)
		}
	}

	hash = sha256.Sum256([]byte("start"))
	for i := int64(0); i < testlen; i++ {
		if (i+1)%blocklen == 0 {
			bi := new(BlockIndex)
			data := MM1.MainChain.Manager.Get("BlockIndex", "", Int64Bytes(i/blocklen))
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

	MM2 := NewMerkleManager(dbManager, salt[:], 2)

	if MM1.MainChain.MS.Count != MM2.MainChain.MS.Count {
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

	const testLen = 1024

	dbManager := new(database.Manager)
	if err := dbManager.Init("memory", ""); err != nil {
		t.Fatal(err)
	}

	MarkPower := int64(2)
	MarkFreq := int64(math.Pow(2, float64(MarkPower)))
	MarkMask := MarkFreq - 1

	// Set up a MM1 that uses a MarkPower of 2
	salt := sha256.Sum256([]byte("root"))
	MM1 := NewMerkleManager(dbManager, salt[:], MarkPower)

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

	// Check the Indexing
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

func TestBlockIndexes(t *testing.T) {
	const testLen = 10240 //                                         Our iterations

	dbManager := new(database.Manager)                   //          Allocate a memory database
	if err := dbManager.Init("memory", ""); err != nil { //
		t.Fatal(err) //
	} //
	chainID := sha256.Sum256([]byte("one"))          //              Create a MerkleManager with a particular salt
	MM := NewMerkleManager(dbManager, chainID[:], 4) //

	rand.Seed(1)                   //                                Start rand from a particular seed
	blkCnt := int64(1)             //                                This will be our block count
	for i := 0; i < testLen; i++ { //                                Then for the specified number of iterations
		v := rand.Int63()                                        //  Assign to v so we can see the number
		MM.AddHash(sha256.Sum256([]byte(fmt.Sprintf(" %d", v)))) //    if  debugging.  Make a hash with it.
		if rand.Intn(30) == 0 {                                  //  About 1 out of 30 times...
			MM.SetBlockIndex(blkCnt) //                              Pretend this is the end of the block
			blkCnt++                 //                              Of course, the next round the block incs.
			if rand.Intn(100) < 15 { //                                About 15 percent of the time, skip a block
				blkCnt++ //                                            skipping
			} //
		} //
		if rand.Intn(10) == 0 { //                                              1 out of 10 times, add a pending entry
			v = rand.Int63()                                                //  Make the entry (use v to see)
			MM.AddPendingHash(sha256.Sum256([]byte(fmt.Sprintf(" %d", v)))) //  Add the Hash
		} //
	}

	rand.Seed(1)                          //           Here is the trick: Reset rand seed to get the same sequence of
	bi := new(BlockIndex)                 //           random numbers.  Allocate a block to read the BlockIndexChain
	blkIdx := int64(0)                    //           Blocks count up from zero
	pendingIdx := int64(-1)               //           pendingIdx is going to start at -1 (we have none)
	blkCnt = 1                            //           blkCnt is the count of our pretend blocks
	for i := int64(0); i < testLen; i++ { //                                            Now for the same iterations...
		v := rand.Int63()       //                                                      Get v again
		_ = v                   //                                                      (v to see (in debugger))
		if rand.Intn(30) == 0 { //                                                      Same rate
			data := MM.BlkIdxChain.Manager.Get("BlockIndex", "", Int64Bytes(blkIdx)) // Get the next element of
			blkIdx++                                                                 // BlockIndexChain. Inc that cnt
			bi.UnMarshal(data)                                                       // Get the struct
			if bi.BlockIndex != blkCnt ||                                            // Should match our blk count
				bi.PendingIndex != pendingIdx || //                                     Should match our pendingIdx
				bi.MainIndex != i { //                                                  Should match i
				t.Fatal("Didn't see what we expected") //  If any of that isn't true, flag
			} //
			blkCnt++                 //                    count our pretend blocks
			if rand.Intn(100) < 15 { //                    Again, about 15% of the time skip a pretend block
				blkCnt++ //
			} //
		} //
		if rand.Intn(10) == 0 { //                         Again, 1 out of 10 times add a pending entry (inc the count)
			v = rand.Int63() //                            Have to mimic calls to rand exactly, so just make
			pendingIdx++     //                              the call to rand, and increment our count
		} //
	} //
}
