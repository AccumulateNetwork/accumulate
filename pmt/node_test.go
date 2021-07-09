package pmt

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"
)

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
