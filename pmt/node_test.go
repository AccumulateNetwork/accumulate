package pmt

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"testing"
)

// GetLeaf
// Find a leaf node in the given BPT tree.  Take a random path
// (But if you set the seed, you can get the same random path)
func GetPath(bpt *BPT) (path []*Node) {
	bpt.Update()
	here := bpt.Root //                                 Find a leaf node
	for {
		path = append(path, here)
		if rand.Int()&1 == 0 {
			if here.left != nil && here.left.T() == TNode { //   If I have a left child
				here = here.left.(*Node) //                      chase it, and continue
				continue
			}
			if here.right != nil && here.right.T() == TNode { // If I have a right child
				here = here.right.(*Node) //                     chase it, and continue
				continue
			}
		} else {
			if here.right != nil && here.right.T() == TNode { // If I have a left child
				here = here.right.(*Node) //                     chase it, and continue
				continue
			}
			if here.left != nil && here.left.T() == TNode { //   If I have a right child
				here = here.left.(*Node) //                      chase it, and continue
				continue
			}
		}
		break
	}
	return path
}

func TestBPT_Equal(t *testing.T) {

	bpt1 := LoadBpt() // Get a bunch of nodes
	bpt2 := LoadBpt() // Get a bunch of nodes
	fmt.Printf("Max Height %d Max ID %d\n", bpt1.MaxHeight, bpt1.MaxNodeID)

	unique := make(map[int64]int64)

	for i := int64(0); i < 100; i++ {
		rand.Seed(i)
		p1 := GetPath(bpt1)
		rand.Seed(i)
		p2 := GetPath(bpt2)

		leaf := p1[len(p1)-1]
		if leaf.left != nil && leaf.left.T() == TNode {
			t.Error("Last node is no leaf")
		}
		//unique[p1[len(p1)-1].ID] = i

		if len(p1) != len(p2) {
			t.Error("Path is not the same")
		}

		for i, v := range p1 {
			if !v.Equal(p2[i]) {
				t.Errorf("nodes should be equal %d of %d", i, len(p1))
			}
		}
	}

	rand.Seed(0)
	p1 := GetPath(bpt1)
	leaf := p1[len(p1)-1]
	_ = leaf
	var v *Value
	if leaf.left != nil {
		v = leaf.left.(*Value)
	} else if leaf.right != nil {
		v = leaf.right.(*Value)
	} else {
		t.Errorf("nodes with nil left and right paths not allowed")
	}

	bpt1.Insert(v.Key, sha256.Sum256(v.Hash[:]))
	rand.Seed(0)
	var equalCnt, unequalCnt int
	for i := int64(0); i < 100; i++ {
		rand.Seed(i)
		p1 := GetPath(bpt1)
		rand.Seed(i)
		p2 := GetPath(bpt2)

		leaf := p1[len(p1)-1]
		if leaf.left != nil && leaf.left.T() == TNode {
			t.Error("Last node is no leaf")
		}
		unique[p1[len(p1)-1].ID] = i

		if len(p1) != len(p2) {
			t.Error("Path is not the same")
		}

		for i, v := range p1 {
			if !v.Equal(p2[i]) {
				unequalCnt++
			} else {
				equalCnt++
			}
		}
	}

	fmt.Println("unique nodes tested ", len(unique))
	fmt.Println("equal nodes ", equalCnt, " unequal nodes ", unequalCnt)
}

func TestNode_Marshal(t *testing.T) {
	node1 := new(Node)
	node1.Height = 27
	node1.Hash = [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 10, 20}
	node1.ID = 87654
	node1.PreBytes = []byte{0xFF, 0xAA, 0xBB}

	data := node1.Marshal()
	node2 := new(Node)
	data = (node2).UnMarshal(data)

	if len(data) != 0 {
		t.Errorf("All data should be consumed")
	}
	if !node1.Equal(node2) {
		t.Errorf("Failed to unmarshal node correctly")
	}
}
