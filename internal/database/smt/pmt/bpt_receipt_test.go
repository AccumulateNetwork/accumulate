package pmt_test

import (
	"bytes"
	"crypto/sha256"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	. "gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt"
)

// returns true if all the hashes in the BPT are correct
func chk(t *testing.T, entry Entry) bool {
	node, ok := entry.(*BptNode)
	if ok && node.Left != nil && node.Right != nil {
		c := sha256.Sum256(append(node.Left.GetHash(), node.Right.GetHash()...))
		if node.Hash != c {
			return false
		}
		return chk(t, node.Left) && chk(t, node.Right)
	} else if ok && node.Left != nil {
		if !bytes.Equal(node.Hash[:], node.Left.GetHash()) {
			return false
		}
		return chk(t, node.Left)
	} else if ok && node.Right != nil {
		if !bytes.Equal(node.Hash[:], node.Right.GetHash()) {
			return false
		}
		return chk(t, node.Right)
	}
	return true
}

// TestBPT_receipt
// Build a reasonable size BPT, then prove we can create a receipt for every
// element in said BPT.
func TestBPT_receipt(t *testing.T) {
	numberEntries := 50000 //               A pretty reasonable sized BPT

	bpt := NewBPTManager(nil).Bpt        //     Build a BPT
	var keys, values common.RandHash     //     use the default sequence for keys
	values.SetSeed([]byte{1, 2, 3})      //     use a different sequence for values
	for i := 0; i < numberEntries; i++ { // For the number of Entries specified for the BPT
		chainID := keys.NextAList() //      Get a key, keep a list
		value := values.NextA()     //      Get some value (don't really care what it is)
		bpt.Insert(chainID, value)  //      Insert the Key with the value into the BPT
	}

	keyList := append([][]byte{}, keys.List...)                                                   // Get all the keys
	sort.Slice(keyList, func(i, j int) bool { return bytes.Compare(keyList[i], keyList[j]) < 0 }) //
	require.False(t, chk(t, bpt.GetRoot()), "should not work as updates are needed")              //

	require.NoError(t, bpt.Update())

	// Make sure every key in the bpt is sorted from greater to smaller. (really a BPT test)
	require.True(t, chk(t, bpt.GetRoot()), "should work, as updates have been done")

	// Make sure every key we added to the BPT has a valid receipt
	for i := range keys.List { // go through the list of keys
		r := bpt.GetReceipt(keys.GetAElement(i))
		require.NotNil(t, r, "Should not be nil")
		v := r.Validate()
		require.Truef(t, v, "should validate BPT with %d element %d", i+1, i+1)
	}

}
