// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pmt_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
	. "gitlab.com/accumulatenetwork/accumulate/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

func TestSanity(t *testing.T) {
	fakeKey := storage.MakeKey("Account", "foo")
	fakeValue := storage.MakeKey("Value")

	store := memory.NewDB()              // Create a database
	batch := store.Begin(true)           // Start a batch
	defer batch.Discard()                //
	bpt := NewBPTManager(batch)          // Create a BPT manager
	bpt.InsertKV(fakeKey, fakeValue)     // Insert a key, value pair
	require.NoError(t, bpt.Bpt.Update()) // Push the update
	require.NoError(t, batch.Commit())   // Commit the batch
	rootHash1 := bpt.Bpt.RootHash        // Remember the root hash
	nodeHash1 := bpt.Bpt.Root.Hash       //

	batch = store.Begin(false)            // Start a batch
	defer batch.Discard()                 //
	bpt = NewBPTManager(batch)            // Create a BPT manager
	bpt.Bpt.GetRoot()                     // Get the root node
	rootHash2 := bpt.Bpt.RootHash         // Load the root hash
	nodeHash2 := bpt.Bpt.Root.Hash        //
	assert.Equal(t, rootHash1, rootHash2) // They should match
	assert.Equal(t, nodeHash1, nodeHash2) //

	receipt := bpt.Bpt.GetReceipt(fakeKey)      // Create a receipt
	receiptRoot := *(*[32]byte)(receipt.Anchor) //
	assert.Equal(t, rootHash1, receiptRoot)     // The MDRoot should match the BPT root hash
}

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
		fmt.Printf("  %x...%x\n", n.NodeKey[:3], n.NodeKey[31:])
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

func Check(t *testing.T, bpt *BPT, node *BptNode) {

	BIdx := byte(node.Height >> 3) //          Calculate the byte index based on the height of this node in the BPT
	bitIdx := node.Height & 7      //          The bit index is given by the lower 3 bits of the height
	bit := byte(0x80) >> bitIdx    //          The mask starts at the high end bit in the byte, shifted right by the bitIdx

	ht, key, ok := GetHtKey(node.NodeKey)

	require.Truef(t, ok, "Should have a nodeKey key %x ht %d", key, ht)
	require.Truef(t, ht == node.Height, "Height should match nodeKey key %x ht %d", key, ht)

	bpt.LoadNext(BIdx, bit, node, key)

	keyLeft, keyRight, _ := GetChildrenNodeKeys(node.NodeKey)

	if lNode, ok := node.Left.(*BptNode); ok {
		require.Truef(t, keyLeft == lNode.NodeKey, "Left branch is broken key %x ht %d", key, ht)
	}
	if rNode, ok := node.Right.(*BptNode); ok {
		require.Truef(t, keyRight == rNode.NodeKey, "Left branch is broken key %x ht %d", key, ht)
	}

	switch {
	case node.Left != nil && node.Right != nil:
		left := GetHash(node.Left)
		right := GetHash(node.Right)
		concat := append(left, right...)
		hash := sha256.Sum256(concat)
		if !bytes.Equal(node.Hash[:], hash[:]) {
			fmt.Println("x")
		}
		require.True(t, bytes.Equal(node.Hash[:], hash[:]), "Hash is wrong")
	case node.Left != nil:
		require.True(t, bytes.Equal(node.Hash[:], GetHash(node.Left)), "Hash is wrong")
	case node.Right != nil:
		require.True(t, bytes.Equal(node.Hash[:], GetHash(node.Right)), "Hash is wrong")
	}

	if lNode, ok := node.Left.(*BptNode); ok {
		Check(t, bpt, lNode)
	}
	if rNode, ok := node.Right.(*BptNode); ok {
		Check(t, bpt, rNode)
	}
}

// TestManager
// Do a pretty complete test of adding elements to a BPT, store the state of the BPT,
// Load the state of the BPT, update the state of the BPT, and cycle that a good number of times.
// Then check that all the elements we put in the BPT are in the BPT
// Then make sure keys are not in the BPT are not found in the BPT.
// Exercise GetRange and Get in these tests.  Test includes a key before any keys in the
// in the BPT and a key after the end of the keys in the BPT
func TestManager(t *testing.T) {
	c := 100 // How many load cycles
	d := 10  // How many key/values to be put in the BPT per cycle

	var keys, values common.RandHash // hash sequences

	store := memory.NewDB()              // use a memory database
	storeTx := store.Begin(true)         // and begin its use.
	bptManager := NewBPTManager(storeTx) // Create a BptManager.  We will create a new one each cycle.

	for i := 0; i < c; i++ { //             For each cycle

		for j := 0; j < d; j++ { //         Stuff in our key values
			k := keys.NextAList()     //    Key a list of the keys
			v := values.NextA()       //
			bptManager.InsertKV(k, v) //
		}
		require.NoError(t, bptManager.Bpt.Update())
		Check(t, bptManager.Bpt, bptManager.Bpt.GetRoot())
		bptManager = NewBPTManager(storeTx)
	}
	var first, place, last [32]byte
	copy(first[:], []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
	_, first = bptManager.Bpt.GetRange(first, 1)
	place = first
	for {
		a, key := bptManager.Bpt.GetRange(place, 100)
		if len(a) == 0 {
			last = place
			break
		}
		place = key
	}

	//fmt.Printf("%x-%x\n", first, last)

	for i := range keys.List {
		_, _, found := bptManager.Bpt.Get(bptManager.Bpt.GetRoot(), keys.GetAElement(i))
		require.Truef(t, found, "Must find every key put into the BPT. Failed at %d", i)
	}

	bptManager = NewBPTManager(storeTx)
	var sb, eb bool
	for i := 0; !sb || !eb; i++ {
		key := keys.NextA()
		_, _, found := bptManager.Bpt.Get(bptManager.Bpt.GetRoot(), key)
		require.Falsef(t, found, "Should not find keys not put into the BPT. Failed at %d", i)
		if bytes.Compare(first[:], key[:]) < 0 {
			sb = true
			//		fmt.Printf("%d first: %x key: %x\n", i, first, key)
		}
		if bytes.Compare(last[:], key[:]) > 0 {
			eb = true
			//		fmt.Printf("%d last:  %x key: %x\n", i, last, key)
		}
	}
}

func TestManagerSeries(t *testing.T) {

	// A set of key value pairs.  We are going to set 100 of them,
	// Then update those value over a running test.
	SetOfValues := make(map[[32]byte][32]byte) // Keep up with key/values we add
	d := 615                                   // Add 100 entries each pass.

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
						SetOfValues[k] = sha256.Sum256(v[:]) //nolint:rangevarref // Update the value by hashing it
					}

					for k := 0; k < d; k++ { // Now add d new entries
						key := sha256.Sum256([]byte(fmt.Sprintf("k key %d %d %d %d", h, i, j, k)))   // use h,i,j,k to make entry unique
						value := sha256.Sum256([]byte(fmt.Sprintf("v key %d %d %d %d", h, i, j, k))) // Same. (k,v) makes key != value
						SetOfValues[key] = value                                                     // Set the key value
						bptManager.InsertKV(key, value)                                              // Add to BPT
					}
				}
				priorRoot := bptManager.GetRootHash()          //        Get the prior root (cause no update yet)
				require.NoError(t, bptManager.Bpt.Update())    //        Update the value
				currentRoot := bptManager.GetRootHash()        //        Get the Root Hash.  Note this is in memory
				if bytes.Equal(priorRoot[:], currentRoot[:]) { //        Prior should be different, cause we added stuff
					t.Error("added stuff, hash should not be equal") //
				} //
				//fmt.Printf("%x %x\n", priorRoot, currentRoot)
				previous = currentRoot //                                Make previous track current state of database
				require.NoError(t, bptManager.Bpt.Update())
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
		storeTx := store.Begin(true)         //
		bptManager := NewBPTManager(storeTx) //

		for j := 0; j < i; j++ {
			k, v := keys.NextAList(), values.NextA()
			bptManager.Bpt.Insert(k, v)
		}

		require.NoError(t, bptManager.Bpt.Update())
		require.Nil(t, storeTx.Commit(), "Should be able to commit the data")

		storeTx = store.Begin(true)
		bptManager = NewBPTManager(storeTx)
		for i, v := range keys.List {
			k := keys.GetAElement(i)
			_, entry, found := bptManager.Bpt.Get(bptManager.Bpt.GetRoot(), k)
			require.NotNilf(t, entry, "Must find all the keys we put into the BPT: node returned is nil %d", i)
			require.Truef(t, found, "Must find all keys in BPT: key index = %d", i)
			value, ok := (*entry).(*Value)
			require.Truef(t, ok, "Must find all the keys we put into the BPT: Key not found %d", i)
			require.Truef(t, bytes.Equal(value.Key[:], v), "Must find all the keys we put into the BPT; Value != key %d", i)
		}

		for i := range keys.List {
			bptManager.Bpt.Insert(keys.GetAElement(i), values.NextA())
		}

		require.NoError(t, bptManager.Bpt.Update())
		require.Nil(t, storeTx.Commit(), "Should be able to commit the data")

		storeTx = store.Begin(true)
		bptManager = NewBPTManager(storeTx)
		for i, v := range keys.List {
			_, entry, found := bptManager.Bpt.Get(bptManager.Bpt.GetRoot(), keys.GetAElement(i))
			value, ok := (*entry).(*Value)
			require.Truef(t, ok && found, "Must find all the keys we put into the BPT: Key not found %i")
			require.Truef(t, bytes.Equal(value.Key[:], v), "Must find all the keys we put into the BPT; Value != key %i", i)
		}

	}
}

func TestBptGet(t *testing.T) {
	numberTests := 50000
	var keys, values common.RandHash
	values.SetSeed([]byte{1, 3, 4}) // Let keys default the seed, make values different
	bpt := NewBPTManager(nil).Bpt
	for i := 0; i < numberTests; i++ {
		k := keys.NextAList()
		v := values.NextA()
		//fmt.Printf("Add  k,v =%x : %x\n",k[:3],v[:3])
		bpt.Insert(k, v)
	}

	// Update all elements in the BPT
	for i := 0; i < numberTests; i++ {
		bpt.Insert(keys.GetAElement(i), values.NextAList())
	}

	require.NoError(t, bpt.Update())

	for i := 0; i < numberTests; i++ {
		idx := i
		k := keys.GetAElement(idx)
		v := values.List[idx]
		node, entry, found := bpt.Get(bpt.GetRoot(), k)
		require.Truef(t, found, "Should find all keys added. idx=%d", idx)
		require.NotNilf(t, node, "Should return a node. idx=%d", idx)
		require.NotNilf(t, *entry, "Should return a value. idx=%d", idx)
		value := (*entry).(*Value)
		//fmt.Printf("Find k,v =%x : %x got %x \n",k[:3],v[:3],value.Key[:3])
		require.Truef(t, bytes.Equal(value.Hash[:], v), "value not expected for idx=%d", idx)
	}

	for i := 0; i < numberTests/10; i++ {
		k := keys.NextA()
		node, _, found := bpt.Get(bpt.GetRoot(), k)
		require.Falsef(t, found, "Should not find a value for a random key idx:=%d", i)
		BIdx := node.Height >> 3 // Get the Byte Index
		require.Truef(t, bytes.Equal(k[:BIdx], node.NodeKey[:BIdx]),
			"Key %x height %d should lead to NodeKey %x", k[:BIdx], node.Height, node.NodeKey[:BIdx])

	}
}
