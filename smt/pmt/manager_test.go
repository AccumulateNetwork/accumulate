package pmt

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

var _ = PrintNode // Avoids highlighting that PrintNode isn't used.  It is useful for debugging.
// PrintNode
// Debugging aid that dumps the BPT tree to allow inspection of the BPT by the
// developer.  Too foamy to use all the time.
func PrintNode(h int, e Entry) {
	for i := 0; i < h; i++ {
		print("_")
	}
	switch {
	case e == nil:
		println("nil")
	case e.T() == TNode:
		n := e.(*BptNode)
		fmt.Printf("  %x...%x\n", n.BBKey[:3], n.BBKey[31:])
		PrintNode(n.Height, n.Left)
		PrintNode(n.Height, n.Right)
	case e.T() == TValue:
		v := e.(*Value)
		fmt.Printf("  k %x v %x\n", v.Key[:5], v.Hash[:5])
	case e.T() == TNotLoaded:
		if h&7 > 0 {
			println("  **** Bad Not Loaded ****")
		} else {
			println("  <not loaded>")
		}
	default:
		panic(fmt.Sprintf("Really? %d ", e.T()))
	}

}

func TestManager(t *testing.T) {

	d := 100000

	store := memory.NewDB()
	storeTx := store.Begin()
	bptManager := NewBPTManager(storeTx)
	for i := 0; i < d; i++ {
		key := sha256.Sum256([]byte(fmt.Sprintf("0 key %d", i)))
		value := sha256.Sum256([]byte(fmt.Sprintf("0 key %d", i)))
		bptManager.InsertKV(key, value)
	}
	bptManager.Bpt.Update()
	//PrintNode(0, bptManager.Bpt.Root)
	bptManager = NewBPTManager(storeTx)
	//PrintNode(0, bptManager.Bpt.Root)
	for i := 0; i < d; i++ {
		key := sha256.Sum256([]byte(fmt.Sprintf("1 key %d", i)))
		value := sha256.Sum256([]byte(fmt.Sprintf("1 key %d", i)))
		bptManager.InsertKV(key, value)
	}
}

func TestManagerSeries(t *testing.T) {

	// A set of key value pairs.  We are going to set 100 of them,
	// Then update those value over a running test.
	SetOfValues := make(map[[32]byte][32]byte) // Keep up with key/values we add
	d := 100                                   // Add 100 entries each pass.

	store := memory.NewDB()
	storeTx := store.Begin()

	var previous [32]byte    // Previous final root
	for h := 0; h < 3; h++ { // Run our test 3 times, each time killing one manager, building another which must
		{ //                     get its state from "disk"
			bptManager := NewBPTManager(storeTx) // Get manager from disk.  First time empty, and more elements each pass
			current := bptManager.GetRootHash()  // Check that previous == current, on every pass
			if !bytes.Equal(previous[:], current[:]) {
				t.Errorf("previous %x should be the same as current %x", previous, current)
			}
			for i := 0; i < 3; i++ { // Add stuff in 3 passes
				for j := 0; j < 3; j++ { // Add more stuff in 3 more passes
					for k, v := range SetOfValues { // Update any existing entries
						SetOfValues[k] = sha256.Sum256(v[:]) // Update the value by hashing it
					}

					for k := 0; k < d; k++ { // Now add d new entries
						key := sha256.Sum256([]byte(fmt.Sprintf("k key %d %d %d %d", h, i, j, k)))   // use h,i,j,k to make entry unique
						value := sha256.Sum256([]byte(fmt.Sprintf("v key %d %d %d %d", h, i, j, k))) // Same. (k,v) makes key != value
						SetOfValues[key] = value                                                     // Set the key value
						bptManager.InsertKV(key, value)                                              // Add to BPT
					}
				}
				priorRoot := bptManager.GetRootHash()          //        Get the prior root (cause no update yet)
				bptManager.Bpt.Update()                        //        Update the value
				currentRoot := bptManager.GetRootHash()        //        Get the Root Hash.  Note this is in memory
				if bytes.Equal(priorRoot[:], currentRoot[:]) { //        Prior should be different, cause we added stuff
					t.Error("added stuff, hash should not be equal") //
				} //
				//fmt.Printf("%x %x\n", priorRoot, currentRoot)
				previous = currentRoot //                                Make previous track current state of database
			}
		}
		bptManager := NewBPTManager(storeTx)    //  One more check that previous is the same as current when
		currentRoot := bptManager.GetRootHash() //  we build a new BPTManager from the database
		//fmt.Printf("=> %x %x\n", previous, currentRoot)
		if !bytes.Equal(previous[:], currentRoot[:]) { //
			t.Error("loading the BPT should have the same root as previous root") //
		}
	}
}

func TestManagerPersist(t *testing.T) {

	numberTest := 50

	for i := 12; i < numberTest; i++ {

		var keys, values common.RandHash   // Set up hash generation for keys and values
		keys.SetSeed([]byte{1, 2, 3, 4})   // Seed keys differently than values, so they have
		values.SetSeed([]byte{5, 6, 7, 8}) // a different set of hashes

		store := memory.NewDB()              // Set up a memory DB
		storeTx := store.Begin()             //
		bptManager := NewBPTManager(storeTx) //

		for j := 0; j < i; j++ {
			k, v := keys.NextAList(), values.NextA()
			bptManager.Bpt.Insert(k, v)
		}

		bptManager.Bpt.Update()
		require.Nil(t, storeTx.Commit(), "Should be able to commit the data")

		storeTx = store.Begin()
		bptManager = NewBPTManager(storeTx)
		for i, v := range keys.List {
			k := keys.GetAElement(i)
			_, entry := bptManager.Bpt.Get(bptManager.Bpt.Root, k)
			require.NotNilf(t, entry, "Must find all the keys we put into the BPT: node returned is nil %d", i)
			value, ok := (*entry).(*Value)
			require.Truef(t, ok, "Must find all the keys we put into the BPT: Key not found %d", i)
			require.Truef(t, bytes.Equal(value.Key[:], v), "Must find all the keys we put into the BPT; Value != key %d", i)
		}

		for i := range keys.List {
			bptManager.Bpt.Insert(keys.GetAElement(i), values.NextA())
		}

		bptManager.Bpt.Update()
		require.Nil(t, storeTx.Commit(), "Should be able to commit the data")

		storeTx = store.Begin()
		bptManager = NewBPTManager(storeTx)
		for i, v := range keys.List {
			_, entry := bptManager.Bpt.Get(bptManager.Bpt.Root, keys.GetAElement(i))
			value, ok := (*entry).(*Value)
			require.Truef(t, ok, "Must find all the keys we put into the BPT: Key not found %i")
			require.Truef(t, bytes.Equal(value.Key[:], v), "Must find all the keys we put into the BPT; Value != key %i", i)
		}

	}
}

func TestBptGet(t *testing.T) {
	var keys, values common.RandHash
	values.SetSeed([]byte{1, 3, 4}) // Let keys default the seed, make values different
	bpt := NewBPT()
	for i := 0; i < 5000; i++ {
		bpt.Insert(keys.NextAList(), values.NextAList())
	}

	for i := 0; i < 2; i++ {
		idx := rand.Intn(5000)
		k := keys.GetAElement(idx)
		v := values.List[idx]
		node, entry := bpt.Get(bpt.Root, k)
		require.NotNilf(t, node, "Should return a node. idx=%d", idx)
		require.NotNilf(t, *entry, "Should return a value. idx=%d", idx)
		value := (*entry).(*Value)
		require.Truef(t, bytes.Equal(value.Key[:], v), "value not expected for idx=%d", idx)
	}
}
