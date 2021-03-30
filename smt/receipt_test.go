package smt

import (
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/AccumulateNetwork/SMT/storage/database"
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
	manager := new(MerkleManager)
	manager.Init(dbManager, 4)
	// populate the database
	for i := 0; i < testMerkleTreeSize; i++ {
		v := GetHash(i)
		manager.HashFeed <- v
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

	for len(manager.HashFeed) > 0 {
		time.Sleep(time.Millisecond)
	}
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

	const testMerkleTreeSize = 1024

	// Create a memory based database
	dbManager := new(database.Manager)
	_ = dbManager.Init("memory", "")
	// Create a MerkleManager for the memory database
	manager := new(MerkleManager)
	manager.Init(dbManager, 4)
	// populate the database
	for i := 0; i < testMerkleTreeSize; i++ {
		v := GetHash(i)
		manager.HashFeed <- v
	}

	for len(manager.HashFeed) > 0 {
		time.Sleep(time.Millisecond)
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
