// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package collect

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/lxrand"
)

// tx
// Junk transaction to generate
type tx struct {
	hash   [32]byte
	length int
	data   [2000]byte
	r      lxrand.Sequence
}

// fill
// create data for a transaction
func (t *tx) fill(i int) {
	t.length = t.r.Uint()%500 + 100
	copy(t.data[:], t.r.Slice(t.length))
}

// buildSnapshot
// builds a collect for a snapshot and generates the given number of transactions and
// stuffs them into said snapshot
func buildSnapshot(t *testing.T, numberTx int) *Collect {
	start := time.Now()
	c, err := NewCollect("./snapshotX", true) // Get a collect object for building
	assert.NoError(t, err, "didn't create a Collect Object")

	testTx := new(tx) // Generate transactions for test
	fmt.Println("writing transactions")
	for i := 0; i < numberTx; i++ { //
		if i%100000 == 0 && i > 0 {
			fmt.Printf("   Transactions processed %d in %v\n", i, time.Since(start))
		}
		testTx.fill(i)                  //
		err = c.WriteTx(testTx.data[:]) //
		E(err, "write tx")
	} //
	fmt.Printf("time: %v\n", time.Since(start))
	fmt.Println("sorting Indexes")
	err = c.SortIndexes() //              Sort all the transactions indexes
	assert.NoError(t, err, "failed to sort transactions")
	fmt.Printf("time: %v\n", time.Since(start))

	return c
}

// Test_hashes
// Puts a pile of hashes into Collect
// Does a BuildHashFile to sort and deduplicate the hashes
// Reads the hashes out using Collect.GetHash()
// Ensures all the hashes we expect are in the file, and no more.
func Test_hashes(t *testing.T) {
	numberHashes := 10000
	var rnd1 lxrand.Sequence // Creates a random sequence
	var rnd2 lxrand.Sequence // All instances of LXRandom produce the same sequence
	c, err := NewCollect("./snapshotX", true)
	assert.NoError(t, err, "failed to create test file")

	fmt.Printf("starting build of %s hashes\n", humanize.Comma(int64(numberHashes)))
	start := time.Now()
	cnt1 := 0
	for i := 0; i < numberHashes/2; i++ {
		cnt1 += 2        // The way we stuff things in, we get 2 entries per loop, sans duplicates
		h := rnd1.Hash() // The first two hashes of the sequence into the hash file
		err = c.WriteHash(h[:])
		E(err, "write hash")
		h = rnd1.Hash()
		err = c.WriteHash(h[:])
		E(err, "write hash")
		h = rnd2.Hash() // Now just add the hashes of the sequence one at a time (duplicates)
		err = c.WriteHash(h[:])
		E(err, "write hash")
	}
	fmt.Printf("collection complete (%v), now sorting:\n", time.Since(start))
	start2 := time.Now()
	err = c.BuildHashFile() // Deduplicate and sort hashes
	E(err, "build hash file")
	fmt.Printf("sorting complete (%v), now reading:\n", time.Since(start2))
	start3 := time.Now()

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
	var rnd3 lxrand.Sequence
	for range m {
		if v, ok := m[rnd3.Hash()]; !ok {
			t.Fatalf("%x hash should be in map", v)
		}
		cnt2++
	}
	// Ensures that the count of
	assert.Truef(t, cnt2 == cnt1, "Should have same number of elements %d %d", cnt1, cnt2)
	c.Close()             // Get rid of all tmp files
	os.Remove(c.Filename) // get rid of the filename
	fmt.Printf("reading complete (%v), total test time: %v\n", time.Since(start3), time.Since(start))
}

// Test_FetchIndex
// Fetch all elements out of the standard Collect object
func Test_FetchIndex(t *testing.T) {
	fmt.Printf("TEST: Index access\n")

	numberTx := 10000
	c := buildSnapshot(t, numberTx)
	c.Close() // Test close and open

	err := c.Open("./snapshotX")
	require.NoError(t, err)
	start := time.Now()

	for i := 0; i < numberTx; i++ { //     Go through all the hashes and check against expected values
		if i%100000 == 0 && i > 0 {
			fmt.Printf("   Transactions processed %d in %v\n", i, time.Since(start))
		}
		tx, hash, err := c.Fetch(i)               //
		assert.NoError(t, err, "failed to fetch") //
		h := sha256.Sum256(tx)                    //
		if !bytes.Equal(h[:], hash) {             //
			fmt.Printf("%8d failed\n", i) //
		}
	}
	fmt.Printf(" Time: %v\n", time.Since(start))
	c.Close()
	os.Remove(c.Filename)
}

// Test_FetchHash
// Tests access by hash by building a snapshot, and ensuring we can pull every entry.
func Test_FetchHash(t *testing.T) {
	fmt.Println("TEST: fetch hash access")

	numberTx := 10000
	var c *Collect
	if err := c.Open("./snapshotX"); err != nil {
		c = buildSnapshot(t, numberTx)
	}

	start := time.Now()
	sum := 0

	var testTx2 tx
	for i := 0; i < numberTx; i++ {
		if i%100000 == 0 && i > 0 {
			fmt.Printf("   Transactions processed %d in %v\n", i, time.Since(start))
		}
		testTx2.fill(i)
		testTx2.hash = sha256.Sum256(testTx2.data[:])
		_, h2, err := c.Fetch(testTx2.hash[:])
		assert.NoErrorf(t, err, "failed to fetch hash %d", i)

		sum += c.GuessCnt

		assert.False(t, err == nil && !bytes.Equal(h2[:], testTx2.hash[:]),
			"Invalid hash found")
	}
	fmt.Printf(" Guess Count %8.3f\n", float64(sum)/float64(numberTx))
	fmt.Printf(" Total time: %v\n", time.Since(start))

	os.Remove(c.out.Name())
	c.Close()
}

func Test_FetchBadHash(t *testing.T) {
	fmt.Println("TEST: fetch bad hash access attempts")
	numberTx := 10000
	c := buildSnapshot(t, numberTx)

	var testTx tx
	start := time.Now()
	sum := 0 // Set up for test

	var r lxrand.Sequence
	r.SetRandomSequence(2424234, [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})

	// None of these hashes in this sequence should be in the Snapshot
	for i := 0; i < numberTx; i++ {
		testTx.hash = r.Hash()
		_, _, err := c.Fetch(testTx.hash) //
		assert.Error(t, err, "found a random hash")
		sum += c.GuessCnt
	}
	fmt.Printf(" Guess Count %8.3f\n", float64(sum)/float64(numberTx))
	fmt.Printf(" Total time: %v\n", time.Since(start))

	os.Remove(c.out.Name())
	c.Close()
}
