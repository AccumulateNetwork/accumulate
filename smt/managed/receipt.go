package managed

import (
	"bytes"
	"fmt"
	"math"
)

// ReceiptNode
// Holds the Left/Right hash combinations for Receipts
type ReceiptNode struct {
	Right bool
	Hash  Hash
}

func (n *ReceiptNode) Copy() *ReceiptNode {
	node := new(ReceiptNode)
	node.Right = n.Right
	node.Hash = n.Hash.Copy()
	return n
}

// Receipt
// Struct builds the Merkle Tree path component of a Merkle Tree Proof.
type Receipt struct {
	Element      Hash           // Hash for which we want a proof.
	ElementIndex int64          //
	Anchor       Hash           // Hash at the point where the Anchor was created.
	AnchorIndex  int64          //
	MDRoot       Hash           // Root expected once all nodes are applied
	Nodes        []*ReceiptNode // Apply these hashes to create an anchor

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
func (r *Receipt) Validate() bool {
	MDRoot := r.Element // To begin with, we start with the object as the MDRoot
	// Now apply all the path hashes to the MDRoot
	for _, node := range r.Nodes {
		// Need a [32]byte to slice
		hash := node.Hash
		if node.Right {
			// If this hash comes from the right, apply it that way
			MDRoot = MDRoot.Combine(Sha256, hash)
			continue
		}
		// If this hash comes from the left, apply it that way
		MDRoot = hash.Combine(Sha256, MDRoot)
	}
	// In the end, MDRoot should be the same hash the receipt expects.
	return MDRoot.Equal(r.MDRoot)
}

// Copy
// Make a copy of this receipt
func (r *Receipt) Copy() *Receipt {
	nr := new(Receipt)
	nr.Element = append([]byte{}, r.Element...)
	nr.Anchor = append([]byte{}, r.Anchor...)
	nr.MDRoot = append([]byte{}, r.MDRoot...)
	for _, n := range r.Nodes {
		nr.Nodes = append(nr.Nodes, n.Copy())
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
	nr := r.Copy()               // Make a copy of the first Receipt
	nr.MDRoot = rm.MDRoot        // The MDRoot will be the one from the appended receipt
	for _, n := range rm.Nodes { // Make a copy and append the Nodes of the appended receipt
		nr.Nodes = append(nr.Nodes, n.Copy())
	}
	return nr, nil
}

func NewReceipt(manager *MerkleManager) *Receipt {
	r := new(Receipt)
	r.manager = manager
	return r
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

// BuildReceipt
// takes the values collected by GetReceipt and flushes out the data structures
// in the Receipt to represent a fully populated version.
func (r *Receipt) BuildReceipt() error {
	state, _ := r.manager.GetAnyState(r.AnchorIndex) // Get the state at the Anchor Index
	state.Trim()                                     // If Pending has any trailing nils, remove them.
	return r.BuildReceiptWith(r.manager.GetIntermediate, r.manager.MS.HashFunction, state)
}

type GetIntermediateFunc func(element, height int64) (l, r Hash, err error)

func (r *Receipt) BuildReceiptWith(getIntermediate GetIntermediateFunc, hashFunc HashFunc, anchorState *MerkleState) error {
	height := int64(1)   // Start the height at 1, because the element isn't part
	r.MDRoot = r.Element // of the nodes collected.  To begin with, the element is the Merkle Dag Root
	stay := true         // stay represents the fact that the proof is already in this column

	// The path from a intermediateHash added to the merkle tree to the anchor
	// starts at the element index and goes to the anchor index.
	// Some indexes have multiple hashes as hashes cascade.  Other
	// indexes are skipped, as they summarized by values in sub
	// merkle trees on the way to the anchor index.
	//
	// This for loop adds the hashes leading up to the highest
	// sub Merkle Tree between the element index and the anchor index
	for idx := r.ElementIndex; idx <= r.AnchorIndex; { // Range over all the elements
		if idx&1 == 0 { // Handle the even cases. Merkle Trees add elements at 0, and combine elements at odd numbers
			idx++        // No point in handling the first intermediateHash, so move to the next column
			stay = false // The proof is lagging in the previous column.
		} else { // The Odd cases hold summary hashes
			lHash, rHash, err := getIntermediate(idx, height) // Get the previous hight left/right hashes
			if err != nil {                                   // Error means end of the column has been reached
				next := int64(math.Pow(2, float64(height-1))) //            Move to the next column 2^(height-1) columns
				idx += next                                   //
				stay = false                                  //            Changing columns
				continue
			}
			r.MDRoot = lHash.Combine(hashFunc, rHash) // We don't have to calculate the MDRoot, but it
			if stay {                                 //   helps debugging.  Check if still in column
				r.Nodes = append(r.Nodes, &ReceiptNode{Hash: lHash, Right: false}) // If so, combine from left
			} else { //                                                     Otherwise
				r.Nodes = append(r.Nodes, &ReceiptNode{Hash: rHash, Right: true}) //  combine from right
			}
			stay = true // By default assume a stay in the column
			height++    // and increment the height.
		}
	}

	// At this point, we have reached the highest sub Merkle Tree root.
	// All we have to do is calculate the set of intermediate hashes that
	// will be combined with the current state of the receipt to match
	// the Merkle Dag Root at the Anchor Index

	if r.AnchorIndex == 0 { // Special case the first entry in a Merkle Tree
		r.MDRoot = r.Element // whose Merkle Dag Root is just the first element
		return nil           // added to the merkle tree
	}

	stay = false // Indicate no elements for the first index have been added
	state := anchorState

	var intermediateHash, lastIH Hash // The intermediateHash tracks the combining of hashes as we go. The
	for i, v := range state.Pending { // last hash computed is the last intermediate Hash used in an anchor
		if v == nil { //                  Skip in Pending until a value is found
			continue
		}
		if intermediateHash == nil { //   If no computations have been started,
			intermediateHash = v.Copy() //   just move the value from pending over
			if height-1 == int64(i) {   // If height is just above entry in pending
				stay = true //                consider processing to continue
			}
			continue
		}
		lastIH = intermediateHash.Copy()                         // compute a new intermediate hash
		intermediateHash = v.Combine(hashFunc, intermediateHash) // Combine Pending with intermediate
		if int64(i) < height-1 {                                 // If not to the proof height, skip
			continue //                                                                 adding to the receipt
		}
		if stay { //                                                     If in the same column
			if int64(i) >= height { //                                    And the proof is at this hight or higher
				r.Nodes = append(r.Nodes, &ReceiptNode{Hash: v, Right: false}) // Add to the receipt
			}
			continue
		}
		r.Nodes = append(r.Nodes, &ReceiptNode{Hash: lastIH, Right: true}) // First time in this column, so add to receipt
		stay = true                                                        // Indicate processing the same column now.

	}
	r.MDRoot = intermediateHash // The Merkle Dag Root is the last intermediate Hash produced.

	return nil
}
