package pmt

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/AccumulateNetwork/accumulated/smt/storage/database"
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
		n := e.(*Node)
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

	dbManager, err := database.NewDBManager("memory", "")
	if err != nil {
		t.Fatal(err)
	}
	bptManager := NewBPTManager(dbManager)
	for i := 0; i < d; i++ {
		key := sha256.Sum256([]byte(fmt.Sprintf("0 key %d", i)))
		value := sha256.Sum256([]byte(fmt.Sprintf("0 key %d", i)))
		bptManager.InsertKV(key, value)
	}
	bptManager.Bpt.Update()
	//PrintNode(0, bptManager.Bpt.Root)
	bptManager = NewBPTManager(dbManager)
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
	SetOfValues := make(map[[32]byte][32]byte)
	d := 100

	dbManager, err := database.NewDBManager("memory", "")
	if err != nil {
		t.Fatal(err)
	}
	{
		bptManager := NewBPTManager(dbManager)

		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				for k, v := range SetOfValues {
					SetOfValues[k] = sha256.Sum256(v[:]) // Update the value
				}

				for k := 0; k < d; k++ {
					key := sha256.Sum256([]byte(fmt.Sprintf("k key %d %d %d", i, j, k)))
					value := sha256.Sum256([]byte(fmt.Sprintf("v key %d %d %d", i, j, k)))
					SetOfValues[key] = value
					bptManager.InsertKV(key, value)
				}
			}
			priorRoot := bptManager.GetRootHash()
			bptManager.Bpt.Update()
			currentRoot := bptManager.GetRootHash()
			fmt.Printf(" prior %x current %x\n", priorRoot, currentRoot)
		}
	}

	bptManager := NewBPTManager(dbManager)
	fmt.Printf(" current %x\n", bptManager.GetRootHash())

}
