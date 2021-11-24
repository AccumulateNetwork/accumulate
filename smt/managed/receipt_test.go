package managed

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulate/smt/storage/database"
	"github.com/dustin/go-humanize"
)

func GetHash(i int) Hash {
	return sha256.Sum256([]byte(fmt.Sprint(i)))
}

func TestReceipt(t *testing.T) {
	const testMerkleTreeSize = 7

	// Create a memory based database
	dbManager := new(database.Manager)
	_ = dbManager.Init("memory", "")
	// Create a MerkleManager for the memory database
	manager, err := NewMerkleManager(dbManager, 4)
	if err != nil {
		fmt.Errorf("did not create a merkle manager: %v", err)
	}
	// populate the database
	for i := 0; i < testMerkleTreeSize; i++ {
		v := GetHash(i)
		manager.AddHash(v)
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
	fmt.Printf("%-7s %x %v\n", "e0123", e0123, e0123[:3])

	fmt.Printf("\n\n %3x %3x %3x %3x %3x %3x %3x\n", e0[:3], e1[:3], e2[:3], e3[:3], e4[:3], e5[:3], e6[:3])
	fmt.Printf("     %3x        %3x        %3x\n", e01[:3], e23[:3], e45[:3])
	fmt.Printf("            %3x\n", e0123[:3])

	fmt.Printf("\n\n %3v %3v   %3v %3v   %3v %3v   %3v\n", e0[:3], e1[:3], e2[:3], e3[:3], e4[:3], e5[:3], e6[:3])
	fmt.Printf("         %3v                %3v                  %3v\n", e01[:3], e23[:3], e45[:3])
	fmt.Printf("                       %3v\n", e0123[:3])

	element := GetHash(0)
	anchor := GetHash(3)

	r := GetReceipt(manager, element, anchor)
	if r == nil {
		t.Fatal("Failed to generate receipt")
	}
	fmt.Println(r.String())
	if !r.Validate() {
		t.Fatal("Receipt fails")
	}
}

func TestReceiptAll(t *testing.T) {
	const testMerkleTreeSize = 500

	// Create a memory based database
	dbManager := new(database.Manager)
	_ = dbManager.Init("memory", "")
	// Create a MerkleManager for the memory database
	manager, err := NewMerkleManager(dbManager, 4)
	if err != nil {
		t.Fatalf("did not create a merkle manager: %v", err)
	}
	// populate the database
	for i := 0; i < testMerkleTreeSize; i++ {
		v := GetHash(i)
		manager.AddHash(v)
	}

	for i := -10; i < testMerkleTreeSize+10; i++ {
		for j := -10; j < testMerkleTreeSize+10; j++ {
			element := GetHash(i)
			anchor := GetHash(j)

			r := GetReceipt(manager, element, anchor)
			if i < 0 || i >= testMerkleTreeSize || //       If i is out of range
				j < 0 || j >= testMerkleTreeSize || //        Or j is out of range
				j < i { //                                    Or if the anchor is before the element
				if r != nil { //                            then you should not be able to generate a receipt
					t.Fatal("Should not be able to generate a receipt")
				}
			} else {
				if r == nil {
					t.Fatal("Failed to generate receipt", i, j)
				}
				if !r.Validate() {
					t.Fatal("Receipt fails for element ", i, " anchor ", j)
				}
			}
		}
	}
}

// GetManager
// Get a manager, and build it with the given MarkPower.  If temp == true
func GetManager(MarkPower int, temp bool, databaseName string, t *testing.T) (manager *MerkleManager, dir string) {

	// Create a memory based database
	dbManager := new(database.Manager)
	if temp {
		var err error
		dir, err = ioutil.TempDir("", "badger")
		if err != nil {
			log.Fatal(err)
		}
		if err = dbManager.Init("badger", dir); err != nil {
			t.Fatal("Failed to create database: ", err)
		}
	} else {
		if err := dbManager.Init("badger", databaseName); err != nil {
			t.Fatal("Failed to create database: ", databaseName)
		}
	}

	// Create a MerkleManager for the memory database
	var err error
	manager, err = NewMerkleManager(dbManager, 2)
	if err != nil {
		fmt.Errorf("did not create a merkle manager: %v", err)
	}
	return manager, dir
}

func PopulateDatabase(manager *MerkleManager, treeSize int64) {
	// populate the database
	start := time.Now()
	startCount := manager.MS.Count
	for i := startCount; i < treeSize; i++ {
		v := GetHash(int(i))
		manager.AddHash(v)
		if i%100000 == 0 {
			seconds := time.Now().Sub(start).Seconds() + 1
			fmt.Println(
				"Entries added ", humanize.Comma(i),
				" at ", (i-startCount)/int64(seconds), " per second.")
		}
	}
}

func GenerateReceipts(manager *MerkleManager, receiptCount int64, t *testing.T) {
	start := time.Now()
	total := new(int64)
	atomic.StoreInt64(total, 0)
	running := new(int64)
	printed := new(int64)

	for i := 0; i < int(manager.MS.Count); i++ {
		go func(i int) {
			atomic.AddInt64(running, 1)
			for j := i; j < int(manager.MS.Count); j++ {
				element := GetHash(i)
				anchor := GetHash(j)

				r := GetReceipt(manager, element, anchor)
				if i < 0 || i >= int(manager.MS.Count) || //       If i is out of range
					j < 0 || j >= int(manager.MS.Count) || //        Or j is out of range
					j < i { //                                    Or if the anchor is before the element
					if r != nil { //                            then you should not be able to generate a receipt
						t.Fatal("Should not be able to generate a receipt")
					}
				} else {
					if r == nil {
						t.Fatal("Failed to generate receipt", i, j)
					}
					if !r.Validate() {
						t.Fatal("Receipt fails for element ", i, " anchor ", j)
					}
					atomic.AddInt64(total, 1)
				}
				t := atomic.LoadInt64(total)
				if t-atomic.LoadInt64(printed) >= 100000 {
					atomic.StoreInt64(printed, t)
					seconds := int64(time.Now().Sub(start).Seconds()) + 1
					fmt.Printf("Element: %7d     Receipts generated: %12s     Rate: %8d/s\n",
						i, humanize.Comma(t), t/seconds)
				}
				if t > receiptCount {
					return
				}
			}
			atomic.AddInt64(running, -1)
		}(i)
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
	manager, dir := GetManager(2, true, "", t)
	defer func() {
		_ = os.RemoveAll(dir)
	}()
	PopulateDatabase(manager, 1400)

	GenerateReceipts(manager, 1500000, t)

}

var badgerReceiptsBig = flag.Bool("badger-receipts-big", false, "Run TestBadgerReceiptsBig")

func TestBadgerReceiptsBig(t *testing.T) {
	if !*badgerReceiptsBig {
		t.Skip("This test takes a long time to run")
	}

	// Don't remove the database (it's not temp)
	manager, _ := GetManager(2, false, "40million", t)

	PopulateDatabase(manager, 1000000*40) // Create a 40 million element Merkle Tree

	GenerateReceipts(manager, 1500000, t)

}
