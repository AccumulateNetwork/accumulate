package smt

import (
	"crypto/sha256"
	"fmt"
	"testing"
	"time"
)

func TestMD(t *testing.T) {

	start := time.Now()
	_ = start

	// We will do all the combinations by hand.  In the test h1 refers to 1 (because it needs to be a variable)
	//
	// We test a partial merkle tree.
	//            1   2   3   4   5   6
	//             \ /     \ /     \ /
	//              12     34       56
	//                \   /
	//                  1234
	//
	//  So what we want to do is hash 1234 with 56  to create the Merkle DAG root
	//            1   2   3   4   5   6
	//             \ /     \ /     \ /
	//             1+2     3+4     5+6
	//                \   /       /
	//                12+34      /
	//                     \    /
	//                    1234+56

	md := new(MS)
	md.InitSha256()
	h1 := sha256.Sum256([]byte{1})
	h2 := sha256.Sum256([]byte{2})
	h3 := sha256.Sum256([]byte{3})
	h4 := sha256.Sum256([]byte{4})
	h5 := sha256.Sum256([]byte{5})
	h6 := sha256.Sum256([]byte{6})
	md.AddToChain(h1)
	md.AddToChain(h2)
	md.AddToChain(h3)
	md.AddToChain(h4)
	md.AddToChain(h5)
	md.AddToChain(h6)
	var h12, h34, h56, h1234, h123456 [32]byte
	h12 = sha256.Sum256(append(h1[:], h2[:]...))
	h34 = sha256.Sum256(append(h3[:], h4[:]...))
	h56 = sha256.Sum256(append(h5[:], h6[:]...))
	h1234 = sha256.Sum256(append(h12[:], h34[:]...))
	h123456 = sha256.Sum256(append(h1234[:], h56[:]...))
	fmt.Printf("%10s %x\n", "h1", h1)
	fmt.Printf("%10s %x\n", "h2", h2)
	fmt.Printf("%10s %x\n", "h3", h3)
	fmt.Printf("%10s %x\n", "h4", h4)
	fmt.Printf("%10s %x\n", "h5", h5)
	fmt.Printf("%10s %x\n", "h6", h6)
	fmt.Printf("%10s %x\n", "h12", h12)
	fmt.Printf("%10s %x\n", "h34", h34)
	fmt.Printf("%10s %x\n", "h56", h56)
	fmt.Printf("%10s %x\n", "h1234", h1234)
	fmt.Printf("%10s %x\n", "h123456", h123456) // This is the correct Merkle DAG

	mdRoot := md.GetMDRoot()
	fmt.Printf("%10s %x\n", "mdRoot", *mdRoot)

	if *mdRoot != h123456 {
		t.Error("the hand calculated Merkle DAG Root should be returned by md.GetMDRoot()")
	}

	MDR := new(MDReceipt)

	//===================== h1 =============
	fmt.Println("build h1")

	MDR.BuildMDReceipt(*md, h1)
	if mdRoot == nil || MDR.MDRoot != *mdRoot {
		if mdRoot != nil {
			t.Errorf("MDRoots don't match.  Expected %x Got %x", *mdRoot, MDR.MDRoot)
		} else {
			t.Errorf("mdRoot of MS is nil")
		}
	}

	if !MDR.Validate() {
		t.Errorf("Receipt fails to validate ")
	}
	//fmt.Printf(" Merkle DAG Root %x\n", MDR.MDRoot)
	if MDR.MDRoot != *mdRoot {
		t.Errorf("Merkle Roots not equal %x %x", MDR.MDRoot, *mdRoot)
	}

	//=================== h2 =============
	MDR.BuildMDReceipt(*md, h2)
	if mdRoot == nil || MDR.MDRoot != *mdRoot {
		if mdRoot != nil {
			t.Errorf("MDRoots don't match.  Expected %x Got %x", *mdRoot, MDR.MDRoot)
		} else {
			t.Errorf("mdRoot of MS is nil")
		}
	}

	if !MDR.Validate() {
		t.Errorf("Receipt fails to validate ")
	}
	//fmt.Printf(" Merkle DAG Root %x\n", MDR.MDRoot)
	if MDR.MDRoot != *mdRoot {
		t.Errorf("Merkle Roots not equal %x %x", MDR.MDRoot, *mdRoot)
	}

	//=================== h3 =============
	MDR.BuildMDReceipt(*md, h3)
	if mdRoot == nil || MDR.MDRoot != *mdRoot {
		if mdRoot != nil {
			t.Errorf("MDRoots don't match.  Expected %x Got %x", *mdRoot, MDR.MDRoot)
		} else {
			t.Errorf("mdRoot of MS is nil")
		}
	}

	if !MDR.Validate() {
		t.Errorf("Receipt fails to validate ")
	}
	//fmt.Printf(" Merkle DAG Root %x\n", MDR.MDRoot)
	if MDR.MDRoot != *mdRoot {
		t.Errorf("Merkle Roots not equal %x %x", MDR.MDRoot, *mdRoot)
	}

	//=================== h4 =============
	MDR.BuildMDReceipt(*md, h4)
	if mdRoot == nil || MDR.MDRoot != *mdRoot {
		if mdRoot != nil {
			t.Errorf("MDRoots don't match.  Expected %x Got %x", *mdRoot, MDR.MDRoot)
		} else {
			t.Errorf("mdRoot of MS is nil")
		}
	}

	if !MDR.Validate() {
		t.Errorf("Receipt fails to validate ")
	}
	//fmt.Printf(" Merkle DAG Root %x\n", MDR.MDRoot)
	if MDR.MDRoot != *mdRoot {
		t.Errorf("Merkle Roots not equal %x %x", MDR.MDRoot, *mdRoot)
	}

	//=================== h5 =============
	MDR.BuildMDReceipt(*md, h5)
	if mdRoot == nil || MDR.MDRoot != *mdRoot {
		if mdRoot != nil {
			t.Errorf("MDRoots don't match.  Expected %x Got %x", *mdRoot, MDR.MDRoot)
		} else {
			t.Errorf("mdRoot of MS is nil")
		}
	}

	if !MDR.Validate() {
		t.Errorf("Receipt fails to validate ")
	}
	//fmt.Printf(" Merkle DAG Root %x\n", MDR.MDRoot)
	if MDR.MDRoot != *mdRoot {
		t.Errorf("Merkle Roots not equal %x %x", MDR.MDRoot, *mdRoot)
	}

	//=================== h6 =============
	MDR.BuildMDReceipt(*md, h6)
	if mdRoot == nil || MDR.MDRoot != *mdRoot {
		if mdRoot != nil {
			t.Errorf("MDRoots don't match.  Expected %x Got %x", *mdRoot, MDR.MDRoot)
		} else {
			t.Errorf("mdRoot of MS is nil")
		}
	}

	if !MDR.Validate() {
		t.Errorf("Receipt fails to validate ")
	}
	//fmt.Printf(" Merkle DAG Root %x\n", MDR.MDRoot)
	if MDR.MDRoot != *mdRoot {
		t.Errorf("merkle Roots not equal %x %x", MDR.MDRoot, *mdRoot)
	}

}
