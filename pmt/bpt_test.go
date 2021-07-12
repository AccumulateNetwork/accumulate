package pmt

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/dustin/go-humanize"
)

const defaultNodeCnt = 1000

// LoadBptCnt
// Load up and return a BPT with NodeCnt number of entries with random
// values.
//
// Note that the same seed value yeilds the same BPT tree.
func LoadBptCnt(seed int64, NodeCnt int64) *BPT {
	rnd := rand.New(rand.NewSource(seed)) //                            Using our own instance of rand makes us concurrency safe
	bpt := NewBPT()                       //                            Allocate a new bpt

	rnd.Seed(seed)               //                                     seed our rand structure
	b := rnd.Int63()             //                                     Create an 8 byte random key seed
	key := sha256.Sum256([]byte{ //                                     Take the sha256 of the byte sequence
		byte(b), byte(b >> 8), byte(b >> 16), byte(b >> 24), //
		byte(b >> 32), byte(b >> 40), byte(b >> 48), byte(b >> 56)}) //

	b = rnd.Int63()               //                                    Create an 8 byte random hash seed
	hash := sha256.Sum256([]byte{ //                                    Take the sha256 of the byte sequence
		byte(b), byte(b >> 8), byte(b >> 16), byte(b >> 24), //
		byte(b >> 32), byte(b >> 40), byte(b >> 48), byte(b >> 56)}) //
	for i := int64(0); i < NodeCnt; i++ { //                            Now for the specified NodeCnt,
		bpt.Insert(key, hash)         //                                Insert our current key and hash
		key = sha256.Sum256(key[:])   //                                Then roll them forward by hashing the
		hash = sha256.Sum256(hash[:]) //                                current key and hash value
	} //
	return bpt //                                                       When all done, return the *BPT
}

// LoadBpt
// Load up a new BPT with an assumed number of entries
func LoadBpt() *BPT { //                                                LoadBpt builds and returns a standard BPT
	return LoadBptCnt(0, defaultNodeCnt) //                             Every call returns the same BPT configuration
}

func TestBPT_Marshal(t *testing.T) {

	bpt1 := LoadBpt()
	bpt2 := LoadBpt()

	if !bpt1.Equal(bpt2) {
		t.Errorf("Two test BPTs that should be equal are not")
	}

	bpt1.Insert(sha256.Sum256([]byte{1}), sha256.Sum256([]byte{2}))
	bpt1.Update()
	if bpt1.Equal(bpt2) {
		t.Errorf("Two test BPTs should not be equal and they are")
	}
}

// TestInsert
// Check to make sure we can add elements to the BPT and get
// out hashes.  And that we can update the BPT
func TestInsert(t *testing.T) {

	const numElements = 1000 // Choose a number of transactions to process

	b := NewBPT()                         // Allocate a BPT
	val := sha256.Sum256([]byte("hello")) // Get a seed hash
	start := time.Now()                   // Time this
	for i := 0; i < 100; i++ {            // Process the elements some number of times
		for j := 0; j < numElements; j++ { // For each element
			key := sha256.Sum256(val[:]) //   compute a key
			b.Insert(key, val)           //   Insert the key value pair
			key, val = val, key          //   And reuse the new key by just swapping.
		}
		fmt.Printf("elements Added: %d nodes to update: %d\n", numElements, len(b.DirtyMap)) // Print some results
		fmt.Println("Max Height: ", b.MaxHeight, "node ID", b.MaxNodeID)                     // Get the Max Height
		bHash := b.Update()                                                                  // Now update all the hashes
		fmt.Printf("Root %x\n", bHash)                                                       // Print the summary hash (intermediate)
		t := float64(time.Now().UnixNano()-start.UnixNano()) / 1000000000                    // Calculate the seconds, with decimal places
		start = time.Now()                                                                   // And print how long it took
		fmt.Printf("seconds: %8.6f\n", t)                                                    // And ... well print it.
	}
}

// TestInsertOrder
// Here we prove that no matter what order the updates, the same updates
// will produce the same BPT.
func TestInsertOrder(t *testing.T) {

	const numElements = 1000 // how many elements we will test

	type kv struct { // Need the elements in a struct so when
		key   [32]byte // the order is mixed up, the same keys
		value [32]byte // follow the same values
	}

	var pair []*kv                            // A slice of kv pairs
	keySeed := sha256.Sum256([]byte("hello")) // Different seeds for keys and values.
	valSeed := sha256.Sum256([]byte("there")) // Not necessary, but it is what I did
	for i := 0; i < numElements; i++ {        // For the number of elements
		p := new(kv)                        //   Get a key / value struct
		p.key = keySeed                     //   key from the keySeed
		p.value = valSeed                   //   value from the value seed
		pair = append(pair, p)              //   stick the key value pair into the slice of kv pairs (so I can replay them)
		keySeed = sha256.Sum256(keySeed[:]) //   move the seed
		valSeed = sha256.Sum256(valSeed[:]) //   move the value
	} // loop and continue

	b := NewBPT()            //        Get a BPT
	start := time.Now()      //        Set the clock
	for _, v := range pair { //        for every pair in the slice, insert them
		b.Insert(v.key, v.value) //    into the PBT
	}
	one := b.Update()                                                  // update the BPT to get the correct summary hash
	tm := float64(time.Now().UnixNano()-start.UnixNano()) / 1000000000 // Get my time in seconds in a float64
	fmt.Printf("seconds: %8.6f\n", tm)                                 // Print my time.
	fmt.Printf("First pass: %x\n", one)                                // Print the summary hash from pass one

	sort.Slice(pair, func(i, j int) bool { //                             Now shuffle the pairs.  Completely different order
		return rand.Int()&1 == 1 //                                       Randimize using the low order bit of the random number generator
	})

	b = NewBPT()             //                                         Get a fresh BPT
	start = time.Now()       //                                         Reset the clock
	for _, v := range pair { //                                         Insert the scrambled pairs
		b.Insert(v.key, v.value) //                                     into the BPT
	} //
	two := b.Update()                                                 // Update the summary hash
	tm = float64(time.Now().UnixNano()-start.UnixNano()) / 1000000000 // Compute the execution time
	fmt.Printf("seconds: %8.6f\n", tm)                                // Print the time
	fmt.Printf("First pass: %x\n", two)                               // Print the summary hash (should be the same)

	first := pair[0]
	sort.Slice(pair, func(i, j int) bool { //                             Now shuffle the pairs.  Completely different order
		return rand.Int()&1 == 1 //                                       Randimize using the low order bit of the random number generator
	})
	if bytes.Equal(pair[0].key[:], first.key[:]) {
		t.Fatal("After shuffle, first entry should not be the same.")
	}

	var last, now [32]byte
	_ = last
	b = NewBPT()             //                                         Get a fresh BPT
	start = time.Now()       //                                         Reset the clock
	for _, v := range pair { //                                         Insert the scrambled pairs
		b.Insert(v.key, v.value) //                                     into the BPT
		now = b.Update()
		if bytes.Equal(now[:], last[:]) {
			t.Fatal("Every Insert should change state.")
		}
	}
	three := b.Update()
	tm = float64(time.Now().UnixNano()-start.UnixNano()) / 1000000000 // Compute the execution time
	fmt.Printf("seconds: %8.6f\n", tm)                                // Print the time
	fmt.Printf("First pass: %x\n", two)                               // Print the summary hash (should be the same)

	if !bytes.Equal(one[:], two[:]) || !bytes.Equal(one[:], three[:]) { // Use the actual go test infrastructure to report possible errors
		t.Fatalf("\n1: %x\n2: %x\n3: %x\n3: %x", one, two, now, three) //                see 'em if they are there
	}

}

// TestUpdateValues
// Here we prove that no matter what order the updates, the same updates
// will produce the same BPT.
func TestUpdateValues(t *testing.T) {

	defer func() {
		if err := recover(); err != nil {
			t.Error(err)
		}
	}()

	const numElements = 1000 // how many elements we will test

	type kv struct { // Need the elements in a struct so when
		key   [32]byte // the order is mixed up, the same keys
		value [32]byte // follow the same values
	}

	var pair []*kv                            // A slice of kv pairs
	keySeed := sha256.Sum256([]byte("hello")) // Different seeds for keys and values.
	valSeed := sha256.Sum256([]byte("there")) // Not necessary, but it is what I did
	for i := 0; i < numElements; i++ {        // For the number of elements
		p := new(kv)                        //   Get a key / value struct
		p.key = keySeed                     //   key from the keySeed
		p.value = valSeed                   //   value from the value seed
		pair = append(pair, p)              //   stick the key value pair into the slice of kv pairs (so I can replay them)
		keySeed = sha256.Sum256(keySeed[:]) //   move the seed
		valSeed = sha256.Sum256(valSeed[:]) //   move the value
	} // loop and continue

	b := NewBPT()            //                Get a BPT
	start := time.Now()      //                Set the clock
	for _, v := range pair { //                for every pair in the slice, insert them
		b.Insert(v.key, v.value) //  it into the PBT
	}

	one := b.Update()                                                  // update the BPT to get the correct summary hash
	tm := float64(time.Now().UnixNano()-start.UnixNano()) / 1000000000 // Get my time in seconds in a float64
	fmt.Printf("seconds: %8.6f\n", tm)                                 // Print my time.
	fmt.Printf("First pass: %x\n", one)                                // Print the summary hash from pass one
	if len(pair) > numElements/2 {
		updatePair := pair[numElements/2]                   //                Pick a pair out in the middle of the list
		updatePair.key = sha256.Sum256(updatePair.value[:]) //                  change the value,
		b.Insert(updatePair.key, updatePair.value)          //                  then insert it into BPT
		onePrime := b.Update()                              //                Update and get the summary hash

		if bytes.Equal(one[:], onePrime[:]) {
			t.Fatalf("one %x should not be the same as onePrime", one)
		}
		fmt.Printf("Prime pass: %x\n", onePrime) // Print the summary hash from pass one
	}
}

func TestUpdateValue(t *testing.T) {
	bft := LoadBpt()
	p := GetPath(bft)
	leaf := p[len(p)-1]

	bft.Update()
	oldHash := bft.Root.Hash

	var v *Value
	_ = v
	if leaf.left != nil {
		v = leaf.left.(*Value)
	} else {
		v = leaf.right.(*Value)
	}
	newH := v.Hash
	newH[0]++
	bft.Insert(v.Key, newH)
	bft.Update()
	if bytes.Equal(bft.Root.Hash[:], oldHash[:]) {
		t.Errorf("root should not be the same after modifying a value")
	}
}

func TestMarshalByteBlock(t *testing.T) {
	bpt1 := LoadBptCnt(1, 100000) // Get a loaded Bpt
	rootData1 := bpt1.Marshal()
	bpt2 := NewBPT() // Get an empty Bpt
	bpt2.UnMarshal(rootData1)
	data1 := bpt1.MarshalByteBlock(bpt1.Root)
	bpt2.UnMarshalByteBlock(bpt2.Root, data1)
	data2 := bpt2.MarshalByteBlock(bpt2.Root)

	if !bytes.Equal(data1, data2) {
		t.Errorf("Should marshal to the same data")
	}
	// The following code provides for dumping data for debugging
	// purposes.

	/*
		if !bytes.Equal(data1, data2) {
			t.Errorf("Should marshal to the same data")

			print(len(data1), " ")
			var ahead int
			for i, v := range data1 {
				if v == data2[i] {
					if ahead > 0 {
						ahead--
					} else {
						print("  ")
					}
				} else {
					fmt.Printf("%02x-%02x", v, data2[i])
					ahead += 5
				}
			}

			println()
			print(len(data1), " ")
			for i, v := range data1 {
				if v == data2[i] {
					print("..")
				} else {
					print(".|")
				}
			}
			println()
		}
		fmt.Printf("%d %0x\n", len(data1), data1)
		fmt.Printf("%d %0x\n", len(data2), data2)
	*/
}

var size, count []int
var blockSize []int
var blockCnt []int
var total int64
var highest int

func walk(bpt *BPT, node *Node) {
	if node.Height >= highest {
		highest = node.Height
		if highest >= len(size) {
			size = append(size, 0)
			count = append(count, 0)
			blockSize = append(blockSize, 0)
			blockCnt = append(blockCnt, 0)
		}
	}
	total++
	data := node.Marshal()
	size[node.Height] += len(data) + 1 // Add one for the type byte
	count[node.Height] += 1

	if node.Height&bpt.mask == 0 {
		data = bpt.MarshalByteBlock(node)
		blockSize[node.Height] += len(data)
		blockCnt[node.Height]++
	}

	if node.left != nil && node.left.T() == TNode {
		walk(bpt, node.left.(*Node))
	}

	if node.right != nil && node.right.T() == TNode {
		walk(bpt, node.right.(*Node))
	}
}

func TestBPTByteSizes(t *testing.T) {
	//t.Skip("Just gives data about distributions in the BPT.")
	println("build BFT")
	bpt := LoadBptCnt(37, 1000000)
	println("walking")
	walk(bpt, bpt.Root)
	maxcnt := 1
	for i, cnt := range count {
		if cnt == 0 {
			continue
		}
		if blockCnt[i] > 0 {
			fmt.Printf("\t%4d\t%10s\t%7.3f%%\t%4s\t%15s\t%9s\t%9s\t%15s\n",
				i,
				humanize.Comma(int64(cnt)),
				float64(cnt)/float64(maxcnt)*100,
				humanize.Comma(int64(size[i]/cnt)),
				humanize.Comma(int64(size[i])),
				humanize.Comma(int64(blockCnt[i])),
				humanize.Comma(int64(blockSize[i]/blockCnt[i])),
				humanize.Comma(int64(blockSize[i])),
			)
		} else {
			fmt.Printf("\t%4d\t%10s\t%7.3f%%\t%4s\t%15s \n",
				i,
				humanize.Comma(int64(cnt)),
				float64(cnt)/float64(maxcnt)*100,
				humanize.Comma(int64(size[i]/cnt)),
				humanize.Comma(int64(size[i])),
			)
		}
		maxcnt += maxcnt
	}
}
