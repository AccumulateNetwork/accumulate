package collect

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Test_hashes
// Puts a pile of hashes into Collect
// Does a BuildHashFile to sort and deduplicate the hashes
// Reads the hashes out using Collect.GetHash()
// Ensures all the hashes we expect are in the file, and no more.
func Test_hashes(t *testing.T) {
	var rnd1 LXRandom // Creates a random sequence
	var rnd2 LXRandom // All instances of LXRandom produce the same sequence
	c, err := NewCollect("./snapshotX", true)
	assert.NoError (t, err, "failed to create test file")

	cnt1 := 0
	for i := 0; i < 10000; i++ {
		cnt1 += 2        // The way we stuff things in, we get 2 entries per loop, sans duplicates
		h := rnd1.Hash() // The first two hashes of the sequence into the hash file
		c.WriteHash(h[:])
		h = rnd1.Hash()
		c.WriteHash(h[:])
		h = rnd2.Hash() // Now just add the hashes of the sequence one at a time (duplicates)
		c.WriteHash(h[:])
	}

	c.BuildHashFile() // Deduplicate and sort hashes

	nh, e2 := c.NumberHashes() // Get the number of deduplicated hashes; should match our count
	assert.NoError(t, e2, "failed to get number of hashes")
	assert.Equal(t, cnt1, nh, "count of hashes should match unique hashes provided")

	m := make(map[[32]byte]int, 100) // Collect all the hashes we get to check completeness
	for {
		h1 := c.GetHash() // Get each hash one at a time
		if h1 == nil {
			break
		}
		var h2 [32]byte
		copy(h2[:], h1)
		m[h2] = 1
	}

	cnt2 := 0 // Get the same sequence, and make sure that's all we got in the map
	var rnd3 LXRandom
	for _, _ = range m {
		if v, ok := m[rnd3.Hash()]; !ok {
			t.Fatalf("%x hash should be in map", v)
		}
		cnt2++
	}
	// Ensures that the count of
	assert.Truef(t, cnt2 == cnt1, "Should have same number of elements %d %d", cnt1, cnt2)
	c.Close()             // Get rid of all tmp files
	os.Remove(c.Filename) // get rid of the filename
}


// tx
// Junk transaction to generate
type tx struct {
	hash   [32]byte
	length int
	data   [2000]byte
	r      LXRandom
}

// fill
// create data for a transaction
func (t *tx) fill(i int) {
	t.length = t.r.Uint()%500 + 100
	copy(t.data[:], t.r.Slice(t.length))
}

func Test_CollectTest(t *testing.T) {
	numberTx := 100000
	start := time.Now()
	c, err := NewCollect("./snapshotX", true) // Get a collect object for building
	assert.NoError(t, err, "didn't create a Collect Object")

	testTx := new(tx) // Generate transactions for test
	fmt.Println("writing transactions")
	for i := 0; i < numberTx; i++ { //
		if i%1000000 == 0 && i > 0 {
			fmt.Printf("Transactions processed %d in %v\n  ", i, time.Since(start))
		}
		testTx.fill(i)            //
		c.WriteTx(testTx.data[:]) //

	} //
	fmt.Printf("time: %v\n", time.Since(start))
	fmt.Println("sorting Indexes")
	err = c.SortIndexes() //              Sort all the transactions indexes
	assert.NoError(t, err, "failed to sort transactions")
	fmt.Printf("time: %v\n", time.Since(start))
	fmt.Println("Close")
	err = c.Close()
	assert.NoError(t, err, "failed on close")
	fmt.Printf("time: %v\n", time.Since(start))

	fmt.Print("\nfile complete\n\nTests\n\n")

	c.Open()

	fmt.Printf("TEST: Index access\n")

	for i := 0; i < numberTx; i++ { //     Go through all the hashes and check against expected values
		tx, hash, err := c.Fetch(i)               //
		assert.NoError(t, err, "failed to fetch") //
		h := sha256.Sum256(tx)                    //
		if !bytes.Equal(h[:], hash) {             //
			fmt.Printf("%8d failed\n", i) //
		}
	}
	fmt.Printf("time: %v\n", time.Since(start))

	fmt.Println("TEST: fetch hash access")
	sum := 0
	var testTx2 tx
	for i := 0; i < numberTx; i++ {
		testTx2.fill(i)
		testTx2.hash = sha256.Sum256(testTx2.data[:])

		_, h2, err := c.Fetch(testTx2.hash[:])
		assert.NoError(t, err, "failed to fetch hash")
		sum += c.GuessCnt

		if !bytes.Equal(h2[:], testTx2.hash[:]) {
			fmt.Printf("%x failed\n", h2[:8])
		}
	}
	fmt.Printf("Total time: %v\n", time.Since(start))

	{
		fmt.Println("TEST: fetch bad hash access attempts")
		sum = 0               // Set up for test
		r := 1423142          // Set up for random string generator
		var bts [32]byte      //
		rs := func() []byte { //
			for i, b := range bts { //  Simple shift xor random
				r := r<<7 ^ r ^ r>>3 // generated string
				bts[i] = b ^ byte(r) // but purely deterministic
			}
			return bts[:]
		}
		for i := 0; i < 1000; i++ {
			testTx.hash = sha256.Sum256(rs())    // Generate a random hash we should never find
			_, _, err := c.Fetch(testTx.hash[:]) //
			assert.Error(t, err, "found a random hash")
			sum += c.GuessCnt
		}
		fmt.Printf("Total time: %v\n", time.Since(start))

		os.Remove(c.Filename)
	}

	fmt.Printf("Guess Count %8.3f", float64(sum)/float64(numberTx))
	fmt.Printf("tests complete")
}


