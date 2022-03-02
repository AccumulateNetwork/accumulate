package pmt

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"

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
	storeTx := store.Begin(true)
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
	storeTx := store.Begin(true)

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
