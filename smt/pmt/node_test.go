package pmt_test

import (
	"crypto/sha256"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/smt/pmt"
)

// GetLeaf
// Find a leaf node in the given BPT tree.  Take a random path
// (But if you set the seed, you can get the same random path)
func GetPath(t *testing.T, bpt *BPT) (path []*BptNode) {
	require.NoError(t, bpt.Update())
	here := bpt.GetRoot() //                                 Find a leaf node
	for {
		path = append(path, here)
		if rand.Int()&1 == 0 {
			if here.Left != nil && here.Left.T() == TNode { //   If I have a Left child
				here = here.Left.(*BptNode) //                      chase it, and continue
				continue
			}
			if here.Right != nil && here.Right.T() == TNode { // If I have a Right child
				here = here.Right.(*BptNode) //                     chase it, and continue
				continue
			}
		} else {
			if here.Right != nil && here.Right.T() == TNode { // If I have a Left child
				here = here.Right.(*BptNode) //                     chase it, and continue
				continue
			}
			if here.Left != nil && here.Left.T() == TNode { //   If I have a Right child
				here = here.Left.(*BptNode) //                      chase it, and continue
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
	//fmt.Printf("Max Height %d Max ID %d\n", bpt1.MaxHeight, bpt1.MaxNodeID)

	for i := int64(0); i < 100; i++ {
		rand.Seed(i)
		p1 := GetPath(t, bpt1)
		rand.Seed(i)
		p2 := GetPath(t, bpt2)

		leaf := p1[len(p1)-1]
		if leaf.Left != nil && leaf.Left.T() == TNode {
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
	p1 := GetPath(t, bpt1)
	leaf := p1[len(p1)-1]
	_ = leaf
	var v *Value
	if leaf.Left != nil {
		v = leaf.Left.(*Value)
	} else if leaf.Right != nil {
		v = leaf.Right.(*Value)
	} else {
		t.Errorf("nodes with nil Left and Right paths not allowed")
	}

	bpt1.Insert(v.Key, sha256.Sum256(v.Hash[:]))
	rand.Seed(0)
	var equalCnt, unequalCnt int
	for i := int64(0); i < 100; i++ {
		rand.Seed(i)
		p1 := GetPath(t, bpt1)
		rand.Seed(i)
		p2 := GetPath(t, bpt2)

		leaf := p1[len(p1)-1]
		if leaf.Left != nil && leaf.Left.T() == TNode {
			t.Error("Last node is no leaf")
		}
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

	//fmt.Println("unique nodes tested ", len(unique))
	//fmt.Println("equal nodes ", equalCnt, " unequal nodes ", unequalCnt)
}

func TestNode_Marshal(t *testing.T) {
	node1 := new(BptNode)
	node1.Height = 27
	node1.NodeKey = [32]byte{0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 14, 15}
	node1.Hash = [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 10, 20}

	data := node1.Marshal()
	node2 := new(BptNode)
	data = (node2).UnMarshal(data)

	if len(data) != 0 {
		t.Errorf("All data should be consumed")
	}
	if !node1.Equal(node2) {
		t.Errorf("Failed to unmarshal node correctly")
	}
}
