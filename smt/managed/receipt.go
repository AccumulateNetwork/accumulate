package managed

import (
	"bytes"
	"fmt"
	"math"
)

// Node
// Holds the Left/Right hash combinations for Receipts
type Node struct {
	Right bool
	Hash  Hash
}

// Receipt
// Struct builds the Merkle Tree path component of a Merkle Tree Proof.
type Receipt struct {
	Element      Hash    // Hash for which we want a proof.
	ElementIndex int64   //
	Anchor       Hash    // Hash at the point where the Anchor was created.
	AnchorIndex  int64   //
	MDRoot       Hash    // Root expected once all nodes are applied
	Nodes        []*Node // Apply these hashes to create an anchor

	// None of the following is persisted.
	manager *MerkleManager // The Merkle Tree Manager from which we are building a receipt
}

// String
// Convert the receipt to a string
func (r *Receipt) String() string {
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("\nElement      %x\n", r.Element))    // Element of proof
	b.WriteString(fmt.Sprintf("ElementIndex %d\n", r.ElementIndex)) // Element of proof
	b.WriteString(fmt.Sprintf("Anchor       %x\n", r.Anchor))       // Anchor point in the Merkle Tree
	b.WriteString(fmt.Sprintf("AnchorIndex  %d\n", r.AnchorIndex))  // Anchor point in the Merkle Tree
	b.WriteString(fmt.Sprintf("MDRoot       %x\n", r.MDRoot))       // MDRoot result of evaluating the receipt path
	working := r.Element                                            // Calculate the receipt path; for debugging print the
	for i, v := range r.Nodes {                                     // intermediate hashes
		r := "L"
		if v.Right {
			r = "R"
			working = Sha256(append(working[:], v.Hash[:]...))
		} else {
			working = Sha256(append(v.Hash[:], working[:]...))
		}
		b.WriteString(fmt.Sprintf(" %10d Apply %s %x working: %x \n", i, r, v.Hash, working))
	}
	return b.String()
}

// Validate
// Take a receipt and validate that the element hash progresses to the
// Merkle Dag Root hash (MDRoot) in the receipt
func (r Receipt) Validate() bool {
	MDRoot := r.Element // To begin with, we start with the object as the MDRoot
	// Now apply all the path hashes to the MDRoot
	for _, node := range r.Nodes {
		// Need a [32]byte to slice
		hash := node.Hash
		if node.Right {
			// If this hash comes from the right, apply it that way
			MDRoot = MDRoot.Combine(Sha256, hash)
		} else {
			// If this hash comes from the left, apply it that way
			MDRoot = hash.Combine(Sha256, MDRoot)
		}
	}
	// In the end, MDRoot should be the same hash the receipt expects.
	return MDRoot.Equal(r.MDRoot)
}

// Copy
// Make a copy of this receipt
func (r Receipt) Copy() *Receipt {
	nr := new(Receipt)
	nr.Element = append([]byte{}, r.Element...)
	nr.Anchor = append([]byte{}, r.Anchor...)
	nr.MDRoot = append([]byte{}, r.MDRoot...)
	for _, n := range r.Nodes {
		node := Node{Right: n.Right, Hash: append([]byte{}, n.Hash...)}
		nr.Nodes = append(nr.Nodes, &node)
	}
	return nr
}

// Combine
// Take a 2nd receipt, attach it to a root receipt, and return the resulting
// receipt.  The idea is that if this receipt is anchored into another chain,
// Then we can create a receipt that proves the element in this receipt all
// the way down to an anchor in the root receipt.
// Note that both this receipt and the root receipt are expected to be good.
func (r *Receipt) Combine(rm *Receipt) (*Receipt, error) {
	if !bytes.Equal(r.MDRoot, rm.Element) {
		return nil, fmt.Errorf("receipts cannot be combined. "+
			"anchor %x doesn't match root merkle tree %x", r.Anchor, rm.Element)
	}
	nr := r.Copy()
	nr.MDRoot = rm.MDRoot
	for _, n := range rm.Nodes {
		nr.Nodes = append(nr.Nodes, n)
	}
	return nr, nil
}

// GetReceipt
// Given a merkle tree and two elements, produce a proof that the element was used to derive the DAG at the anchor
// Note that the element must be added to the Merkle Tree before the anchor, but the anchor can be any element
// after the element, or even the element itself.
func GetReceipt(manager *MerkleManager, element Hash, anchor Hash) (r *Receipt, err error) {
	// Allocate r, the receipt we are building and record our element
	r = new(Receipt)    // Allocate a r
	r.Element = element // Add the element to the r
	r.Anchor = anchor   // Add the anchor hash to the r
	r.manager = manager
	if r.ElementIndex, err = r.manager.GetElementIndex(element); err != nil {
		return nil, err
	}
	if r.AnchorIndex, err = r.manager.GetElementIndex(anchor); err != nil {
		return nil, err
	}

	if r.ElementIndex > r.AnchorIndex ||
		r.ElementIndex < 0 ||
		r.ElementIndex > r.manager.GetElementCount() { // The element must be at the anchorIndex or before
		return nil, fmt.Errorf("invalid indexes for the element %d and anchor %d", r.ElementIndex, r.AnchorIndex)
	}

	if r.ElementIndex == 0 && r.AnchorIndex == 0 { // If this is the first element in the Merkle Tree, we are already done.
		r.MDRoot = element // A Merkle Tree of one element has a root of the element itself.
		return r, nil      // And we are done!
	}

	if err := r.BuildReceipt(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Receipt) BuildReceipt() error {
	height := int64(0)
	r.MDRoot = r.Element

	for idx := r.ElementIndex; idx <= r.AnchorIndex; {
		if idx&1 == 0 {
			idx++
			height++
		} else {
			lHash, rHash, err := r.manager.GetIntermediate(idx, height)
			if err != nil {
				next := int64(math.Pow(2, float64(height-1)))
				idx += next
				continue
			}
			node := new(Node)
			r.MDRoot = lHash.Combine(r.manager.MS.HashFunction, rHash)
			if height == 0 {
				node.Right = false
				node.Hash = lHash
			} else {
				node.Right = true
				node.Hash = rHash
			}
			r.Nodes = append(r.Nodes, node)
			height++
		}
	}

	if r.AnchorIndex == 0 {
		r.MDRoot = r.Element
		return nil
	}

	if r.AnchorIndex&1 == 1 && r.AnchorIndex == r.ElementIndex {
		lHash, rHash, _ := r.manager.GetIntermediate(r.AnchorIndex, 1)
		node := new(Node)
		node.Right = false
		node.Hash = lHash
		r.Nodes = append(r.Nodes, node)
		r.MDRoot = lHash.Combine(r.manager.MS.HashFunction, rHash)
	} else {
		state, _ := r.manager.GetAnyState(r.AnchorIndex)
		state.Trim()
		var hash, last Hash
		for i, v := range state.Pending {
			if hash == nil {
				hash = v
				continue
			}
			if v != nil {
				last = hash
				hash = v.Combine(r.manager.MS.HashFunction, hash)
			}
			if int64(i) >= height-1 {
				node := new(Node)
				node.Right = true
				node.Hash = last
				r.Nodes = append(r.Nodes, node)
			}
		}
		r.MDRoot = hash
	}

	return nil
}
