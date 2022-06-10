package managed_test

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
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
	. "gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func GetHash(i int) Hash {
	return Sha256([]byte(fmt.Sprint(i)))
}

func TestReceipt(t *testing.T) {
	const testMerkleTreeSize = 7
	// Create a memory based database
	store := database.OpenInMemory(nil)
	storeTx := store.Begin(true)

	// Create a MerkleManager for the memory database
	manager, err := NewMerkleManager(database.MerkleDbManager{Batch: storeTx}, 2)
	if err != nil {
		t.Fatalf("did not create a merkle manager: %v", err)
	}
	// populate the database
	for i := 0; i < testMerkleTreeSize; i++ {
		v := GetHash(i)
		require.NoError(t, manager.AddHash(v, false))
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

	r, err1 := GetReceipt(manager, element, anchor)
	if err1 != nil {
		t.Fatal("Failed to generate receipt")
	}
	fmt.Println(r.String())
	if !r.Validate() {
		t.Fatal("Receipt fails")
	}
}

func TestReceiptAll(t *testing.T) {
	cnt := 0
	const testMerkleTreeSize = 150

	store := database.OpenInMemory(nil)
	storeTx := store.Begin(true)
	manager, _ := NewMerkleManager(database.MerkleDbManager{Batch: storeTx}, 2) // MerkleManager

	_ = manager.SetKey(storage.MakeKey("one")) // Populate a database
	var rh common.RandHash                     // A source of random hashes
	for i := 0; i < testMerkleTreeSize; i++ {  // Then for all the hashes for our test
		require.NoError(t, manager.AddHash(rh.NextList(), false)) // Add a hash
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
			r, err := GetReceipt(manager, element, anchor)

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
				if !r.Validate() {
					t.Errorf("Receipt fails for element/anchor [%d %d]\n", i, int64(j))
				}
				//require.Truef(t, bytes.Equal(r.MDRoot, mdRoots[j]),
				//	"Anchors should match %d %d got %x expected %x", i, j, r.MDRoot[:4], mdRoots[j][:4])
			}
		}
	}
	fmt.Println("Ran ", cnt, " tests")
}

func PopulateDatabase(t *testing.T, manager *MerkleManager, treeSize int64) {
	// populate the database
	startCount := manager.MS.Count
	for i := startCount; i < treeSize; i++ {
		v := GetHash(int(i))
		require.NoError(t, manager.AddHash(v, false))
	}
}

func GenerateReceipts(manager *MerkleManager, receiptCount int64, t *testing.T) {
	start := time.Now()
	total := new(int64)
	atomic.StoreInt64(total, 0)
	running := new(int64)
	printed := new(int64)

	for i := 0; i < int(manager.MS.Count); i++ {
		atomic.AddInt64(running, 1)
		for j := i; j < int(manager.MS.Count); j++ {
			element := GetHash(i)
			anchor := GetHash(j)

			r, _ := GetReceipt(manager, element, anchor)
			if i < 0 || i >= int(manager.MS.Count) || //       If (i) is out of range
				j < 0 || j >= int(manager.MS.Count) || //        Or j is out of range
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
				if !r.Validate() {
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

	badger, err := database.OpenBadger(filepath.Join(t.TempDir(), "badger.db"), nil)
	require.NoError(t, err)
	defer badger.Close()

	batch := badger.Begin(true)
	defer batch.Discard()

	manager, err := NewMerkleManager(database.MerkleDbManager{Batch: batch}, 2)
	require.NoError(t, err)

	PopulateDatabase(t, manager, 700)

	GenerateReceipts(manager, 1500, t)

}

func TestReceipt_Combine(t *testing.T) {
	testCnt := int64(50)
	var rh common.RandHash
	var m1, m2 *MerkleManager
	store := database.OpenInMemory(nil)
	storeTx := store.Begin(true)
	m1, err := NewMerkleManager(database.MerkleDbManager{Batch: storeTx}, 2)
	require.NoError(t, err, "should be able to create a new merkle tree manager")
	err = m1.SetKey(storage.MakeKey("m1"))
	require.NoError(t, err, "should be able to set a key")
	m2, err = NewMerkleManager(database.MerkleDbManager{Batch: storeTx}, 2)
	require.NoError(t, err, "should be able to create a new merkle tree manager")
	err = m2.SetKey(storage.MakeKey("m2"))
	require.NoError(t, err, "should be able to set a key")

	for i := int64(0); i < testCnt; i++ {
		require.NoError(t, m1.AddHash(rh.NextList(), false))
		root1 := m1.MS.GetMDRoot()
		require.NoError(t, m2.AddHash(root1, false))
	}
	for i := int64(0); i < testCnt; i++ {
		for j := i; j < testCnt; j++ {
			element, _ := m1.Get(i)
			anchor, _ := m1.Get(j)
			r, _ := GetReceipt(m1, element, anchor)
			state, _ := m1.GetAnyState(j)
			mdRoot := state.GetMDRoot()

			require.Truef(t, bytes.Equal(r.Anchor, mdRoot), "m1 MDRoot not right %d %d", i, j)
			element, _ = m2.Get(i)
			anchor, _ = m2.Get(j)
			r, _ = GetReceipt(m2, element, anchor)
			state, _ = m2.GetAnyState(j)
			mdRoot = state.GetMDRoot()
			require.Truef(t, bytes.Equal(r.Anchor, mdRoot), "m2 MDRoot not right %d %d", i, j)
		}
	}
	for i := int64(0); i < testCnt; i++ {
		for j := i + 1; j < testCnt; j++ {
			element, _ := m1.Get(i)
			anchor, _ := m1.Get(j)
			r1, _ := GetReceipt(m1, element, anchor)
			require.Truef(t, r1.Validate(), "receipt failed %d %d", i, j)
			require.NotNilf(t, r1, "test case i %d j %d failed to create r1", i, j)
			for k := j; k < testCnt; k++ {
				element, _ = m2.Get(j)
				anchor, _ = m2.Get(k)
				r2, _ := GetReceipt(m2, element, anchor)
				require.Truef(t, r2.Validate(), "receipt failed %d %d", i, j, k)
				require.NotNilf(t, r2, "test case i %d j %d k %d failed to create r2", i, j, k)
				r3, err := r1.Combine(r2)
				require.NoErrorf(t, err, "no error expected combining receipts %d %d %d", i, j, k)
				require.Truef(t, r3.Validate(), "combined receipt failed. %d %d %d", i, j, k)
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

	store := database.OpenInMemory(nil)                                     //  Set up a memory db
	storeTx := store.Begin(true)                                            //
	m, err := NewMerkleManager(database.MerkleDbManager{Batch: storeTx}, 2) //
	require.Nil(t, err, "fail NewMerkleManager")

	for _, v := range list { //                Put all the values into the SMT
		require.NoError(t, m.AddHash(v, false), "Error") //
	}

	// We can now generate a receipt
	receipt, err := GetReceipt(m, list[0], list[cnt-1])
	require.Nil(t, err, "fail GetReceipt")
	require.True(t, receipt.Validate(), "Receipt failed")

}
