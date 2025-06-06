// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/lxrand"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
)

func GetHash(i int) []byte {
	return doSha([]byte(fmt.Sprint(i)))
}

func TestReceipt(t *testing.T) {
	const testMerkleTreeSize = 7
	// Create a memory based database
	store := begin()

	// Create a MerkleManager for the memory database
	manager := testChain(store, 2)
	// populate the database
	for i := 0; i < testMerkleTreeSize; i++ {
		v := GetHash(i)
		require.NoError(t, manager.AddEntry(v, false))
		fmt.Printf("e%-6d %x %v\n", i, v, v[:3])
	}
	e0 := GetHash(0)
	e1 := GetHash(1)
	e2 := GetHash(2)
	e3 := GetHash(3)
	e4 := GetHash(4)
	e5 := GetHash(5)
	e6 := GetHash(6)
	e01 := sha256.Sum256(append(e0[:], e1[:]...))
	e23 := sha256.Sum256(append(e2[:], e3[:]...))
	e45 := sha256.Sum256(append(e4[:], e5[:]...))
	e0123 := sha256.Sum256(append(e01[:], e23[:]...))

	fmt.Printf("%-7s %x %v\n", "e01", e01, e01[:3])
	fmt.Printf("%-7s %x %v\n", "e23", e23, e23[:3])
	fmt.Printf("%-7s %x %v\n", "e45", e45, e45[:3])
	fmt.Printf("%-7s %x %v\n", "e0123", e0123, e0123[:3])

	fmt.Printf("\n\n %3x %3x %3x %3x %3x %3x %3x\n", e0[:3], e1[:3], e2[:3], e3[:3], e4[:3], e5[:3], e6[:3])
	fmt.Printf("     %3x        %3x        %3x\n", e01[:3], e23[:3], e45[:3])
	fmt.Printf("            %3x\n", e0123[:3])

	fmt.Printf("\n\n %3v %3v   %3v %3v   %3v %3v   %3v\n", e0[:3], e1[:3], e2[:3], e3[:3], e4[:3], e5[:3], e6[:3])
	fmt.Printf("         %3v                %3v                  %3v\n", e01[:3], e23[:3], e45[:3])
	fmt.Printf("                       %3v\n", e0123[:3])

	element := GetHash(0)
	anchor := GetHash(3)

	r, err1 := getReceipt(manager, element, anchor)
	if err1 != nil {
		t.Fatal("Failed to generate receipt")
	}
	fmt.Println(PrintReceipt(r))
	if !r.Validate(nil) {
		t.Fatal("Receipt fails")
	}
}

// String
// Convert the receipt to a string
func PrintReceipt(r *Receipt) string {
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("\nStart      %x\n", r.Start))    // Start of proof
	b.WriteString(fmt.Sprintf("StartIndex %d\n", r.StartIndex)) // Start of proof
	b.WriteString(fmt.Sprintf("End        %x\n", r.End))        // End point in the Merkle Tree
	b.WriteString(fmt.Sprintf("EndIndex   %d\n", r.EndIndex))   // End point in the Merkle Tree
	b.WriteString(fmt.Sprintf("Anchor     %x\n", r.Anchor))     // Anchor result of evaluating the receipt path
	working := r.Start                                          // Calculate the receipt path; for debugging print the
	for i, v := range r.Entries {                               // intermediate hashes
		r := "L"
		if v.Right {
			r = "R"
			working = doSha(append(working[:], v.Hash[:]...))
		} else {
			working = doSha(append(v.Hash[:], working[:]...))
		}
		b.WriteString(fmt.Sprintf(" %10d Apply %s %x working: %x \n", i, r, v.Hash, working))
	}
	return b.String()
}

func TestReceiptAll(t *testing.T) {
	cnt := 0
	const testMerkleTreeSize = 150

	store := begin()
	manager := testChain(store, 2, "one")     // Populate a database
	var rh common.RandHash                    // A source of random hashes
	for i := 0; i < testMerkleTreeSize; i++ { // Then for all the hashes for our test
		require.NoError(t, manager.AddEntry(rh.NextList(), false)) // Add a hash
	}

	for i := 0; i < testMerkleTreeSize; i++ {
		for j := 0; j < testMerkleTreeSize; j++ {
			//fmt.Println("--------------i,j ", i, ",", j, " ---------------")
			element := rh.Next()
			if i >= 0 && i < testMerkleTreeSize {
				element = rh.List[i]
			}
			anchor := rh.Next()
			if j >= 0 && j < testMerkleTreeSize {
				anchor = rh.List[j]
			}

			cnt++
			r, err := getReceipt(manager, element, anchor)

			if i < 0 || i >= testMerkleTreeSize || //       If i is out of range
				j < 0 || j >= testMerkleTreeSize || //        Or j is out of range
				j < i { //                                    Or if the anchor is before the element
				if r != nil { //                            then you should not be able to generate a receipt
					t.Fatal("Should not be able to generate a receipt")
				}
			} else {
				require.Nilf(t, err, "Failed to get a receipt: %v", err)
				if r == nil {
					t.Fatal("Failed to generate receipt", i, j)
				}
				if !r.Validate(nil) {
					t.Errorf("Receipt fails for element/anchor [%d %d]\n", i, int64(j))
				}
				//require.Truef(t, bytes.Equal(r.MDRoot, mdRoots[j]),
				//	"Anchors should match %d %d got %x expected %x", i, j, r.MDRoot[:4], mdRoots[j][:4])
			}
		}
	}
	fmt.Println("Ran ", cnt, " tests")
}

func PopulateDatabase(t *testing.T, manager *Chain, treeSize int64) {
	// populate the database
	head, err := manager.Head().Get()
	require.NoError(t, err)
	startCount := head.Count
	for i := startCount; i < treeSize; i++ {
		v := GetHash(int(i))
		require.NoError(t, manager.AddEntry(v, false))
	}
}

func GenerateReceipts(manager *Chain, receiptCount int64, t *testing.T) {
	start := time.Now()
	total := new(int64)
	atomic.StoreInt64(total, 0)
	running := new(int64)
	printed := new(int64)

	head, err := manager.Head().Get()
	require.NoError(t, err)
	for i := 0; i < int(head.Count); i++ {
		atomic.AddInt64(running, 1)
		for j := i; j < int(head.Count); j++ {
			element := GetHash(i)
			anchor := GetHash(j)

			r, _ := getReceipt(manager, element, anchor)
			if i < 0 || i >= int(head.Count) || //       If (i) is out of range
				j < 0 || j >= int(head.Count) || //        Or j is out of range
				j < i { //                                    Or if the anchor is before the element
				if r != nil { //                            then you should not be able to generate a receipt
					t.Error("Should not be able to generate a receipt")
					return
				}
			} else {
				if r == nil {
					t.Error("Failed to generate receipt", i, j)
					return
				}
				if !r.Validate(nil) {
					t.Error("Receipt fails for element ", i, " anchor ", j)
					return
				}
				atomic.AddInt64(total, 1)
			}
			t := atomic.LoadInt64(total)
			if t-atomic.LoadInt64(printed) >= 100000 {
				atomic.StoreInt64(printed, t)
				seconds := int64(time.Since(start).Seconds()) + 1
				fmt.Printf("Element: %7d     Receipts generated: %12s     Rate: %8d/s\n",
					i, humanize.Comma(t), t/seconds)
			}
			if t > receiptCount {
				return
			}
		}
		atomic.AddInt64(running, -1)
		if atomic.LoadInt64(total) > receiptCount {
			break
		}
		if atomic.LoadInt64(running) > 5 {
			for atomic.LoadInt64(running) > 2 {
				time.Sleep(time.Millisecond)
			}
		}
	}
	for atomic.LoadInt64(running) > 0 {
	}
}

func TestBadgerReceipts(t *testing.T) {
	// acctesting.SkipCI(t, "flaky")
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping test: running CI: flaky")
	}

	badger, err := badger.New(filepath.Join(t.TempDir(), "badger.db"))
	require.NoError(t, err)
	defer badger.Close()

	batch := badger.Begin(nil, true)
	defer batch.Discard()

	manager := testChain(keyvalue.RecordStore{Store: batch}, 2)

	PopulateDatabase(t, manager, 700)

	GenerateReceipts(manager, 1500, t)

}

func TestReceipt_Combine(t *testing.T) {
	testCnt := int64(50)
	var rh common.RandHash
	store := begin()
	m1 := testChain(store, 2, "m1")
	m2 := testChain(store, 2, "m2")

	for i := int64(0); i < testCnt; i++ {
		require.NoError(t, m1.AddEntry(rh.NextList(), false))
		head, err := m1.Head().Get()
		require.NoError(t, err)
		root1 := head.Anchor()
		require.NoError(t, m2.AddEntry(root1, false))
	}
	for i := int64(0); i < testCnt; i++ {
		for j := i; j < testCnt; j++ {
			element, _ := m1.Entry(i)
			anchor, _ := m1.Entry(j)
			r, _ := getReceipt(m1, element, anchor)
			state, _ := m1.StateAt(j)
			mdRoot := state.Anchor()

			require.Truef(t, bytes.Equal(r.Anchor, mdRoot), "m1 MDRoot not right %d %d", i, j)
			element, _ = m2.Entry(i)
			anchor, _ = m2.Entry(j)
			r, _ = getReceipt(m2, element, anchor)
			state, _ = m2.StateAt(j)
			mdRoot = state.Anchor()
			require.Truef(t, bytes.Equal(r.Anchor, mdRoot), "m2 MDRoot not right %d %d", i, j)
		}
	}
	for i := int64(0); i < testCnt; i++ {
		for j := i + 1; j < testCnt; j++ {
			element, _ := m1.Entry(i)
			anchor, _ := m1.Entry(j)
			r1, _ := getReceipt(m1, element, anchor)
			require.Truef(t, r1.Validate(nil), "receipt failed %d %d", i, j)
			require.NotNilf(t, r1, "test case i %d j %d failed to create r1", i, j)
			for k := j; k < testCnt; k++ {
				element, _ = m2.Entry(j)
				anchor, _ = m2.Entry(k)
				r2, _ := getReceipt(m2, element, anchor)
				require.Truef(t, r2.Validate(nil), "receipt failed %d %d", i, j, k)
				require.NotNilf(t, r2, "test case i %d j %d k %d failed to create r2", i, j, k)
				r3, err := r1.Combine(r2)
				require.NoErrorf(t, err, "no error expected combining receipts %d %d %d", i, j, k)
				require.Truef(t, r3.Validate(nil), "combined receipt failed. %d %d %d", i, j, k)
			}
		}
	}

}

// TestReceiptSimple
// Make a simple SMT from a list of hashes and values
func TestReceiptSimple(t *testing.T) {

	var cnt = 5000 //                          number of tests

	var rh common.RandHash     //              Create a list of values
	var list [][]byte          //
	for i := 0; i < cnt; i++ { //              Create a value for every numberValues
		list = append(list, rh.Next()) //
	}

	store := begin()         //  Set up a memory db
	m := testChain(store, 2) //

	for _, v := range list { //                Put all the values into the SMT
		require.NoError(t, m.AddEntry(v, false), "Error") //
	}

	// We can now generate a receipt
	receipt, err := getReceipt(m, list[0], list[cnt-1])
	require.Nil(t, err, "fail GetReceipt")
	require.True(t, receipt.Validate(nil), "Receipt failed")

}

func TestReceipt_Combine_Multiple(t *testing.T) {
	var rand lxrand.Sequence

	r1 := &Receipt{
		Start:   rand.Slice(32),
		Entries: []*ReceiptEntry{{Hash: rand.Slice(32)}},
	}
	r1.Anchor = r1.Entries[0].apply(r1.Start)

	r2 := &Receipt{
		Start:   r1.Anchor,
		Entries: []*ReceiptEntry{{Hash: rand.Slice(32)}},
	}
	r2.Anchor = r2.Entries[0].apply(r2.Start)

	r3 := &Receipt{
		Start:   r2.Anchor,
		Entries: []*ReceiptEntry{{Hash: rand.Slice(32)}},
	}
	r3.Anchor = r3.Entries[0].apply(r3.Start)

	r, err := r1.Combine(r2, r3)
	require.NoError(t, err)
	require.Equal(t, r1.Start, r.Start)
	require.Equal(t, r3.Anchor, r.Anchor)
}
