package merkle

import (
	"bytes"
	"fmt"
	"math"
)

func (n *ReceiptEntry) apply(hash []byte) []byte {
	if n.Right {
		// If this hash comes from the right, apply it that way
		return combineHashes(hash, n.Hash)
	}
	// If this hash comes from the left, apply it that way
	return combineHashes(n.Hash, hash)
}

// Validate
// Take a receipt and validate that the element hash progresses to the
// Merkle Dag Root hash (MDRoot) in the receipt
func (r *Receipt) Validate() bool {
	MDRoot := r.Start // To begin with, we start with the object as the MDRoot
	// Now apply all the path hashes to the MDRoot
	for _, node := range r.Entries {
		MDRoot = node.apply(MDRoot)
	}
	// In the end, MDRoot should be the same hash the receipt expects.
	return bytes.Equal(MDRoot, r.Anchor)
}

// Contains returns true if the 2nd receipt is equal to or contained within the
// first.
func (r *Receipt) Contains(other *Receipt) bool {
	hashSelf, hashOther := r.Start, other.Start
	var posSelf int
	for !bytes.Equal(hashSelf, hashOther) {
		if posSelf >= len(r.Entries) {
			return false
		}
		hashSelf = r.Entries[posSelf].apply(hashSelf)
		posSelf++
	}

	for _, entry := range other.Entries {
		if posSelf >= len(r.Entries) {
			return false
		}

		hashSelf = r.Entries[posSelf].apply(hashSelf)
		hashOther = entry.apply(hashOther)
		posSelf++
		if !bytes.Equal(hashSelf, hashOther) {
			return false
		}
	}

	return true
}

// Combine
// Take a 2nd receipt, attach it to a root receipt, and return the resulting
// receipt.  The idea is that if this receipt is anchored into another chain,
// Then we can create a receipt that proves the element in this receipt all
// the way down to an anchor in the root receipt.
// Note that both this receipt and the root receipt are expected to be good.
func (r *Receipt) Combine(rm *Receipt) (*Receipt, error) {
	if !bytes.Equal(r.Anchor, rm.Start) {
		return nil, fmt.Errorf("receipts cannot be combined: anchor %x doesn't match root merkle tree %x", r.Anchor, rm.Start)
	}
	nr := r.Copy()                 // Make a copy of the first Receipt
	nr.Anchor = rm.Anchor          // The MDRoot will be the one from the appended receipt
	for _, n := range rm.Entries { // Make a copy and append the Nodes of the appended receipt
		nr.Entries = append(nr.Entries, n.Copy())
	}
	return nr, nil
}

// CombineReceipts combines multiple receipts.
func CombineReceipts(receipts ...*Receipt) (*Receipt, error) {
	r := receipts[0]
	var err error
	for _, s := range receipts[1:] {
		r, err = r.Combine(s)
		if err != nil {
			return nil, fmt.Errorf("failed to combine receipts: %v", err)
		}
	}

	return r, nil
}

type GetIntermediateFunc func(element, height int64) (l, r []byte, err error)

// BuildReceipt
// takes the values collected by GetReceipt and flushes out the data structures
// in the receipt to represent a fully populated version.
func (r *Receipt) Build(getIntermediate GetIntermediateFunc, anchorState *State) error {
	height := int64(1) // Start the height at 1, because the element isn't part
	r.Anchor = r.Start // of the nodes collected.  To begin with, the element is the Merkle Dag Root
	stay := true       // stay represents the fact that the proof is already in this column

	// The path from a intermediateHash added to the merkle tree to the anchor
	// starts at the element index and goes to the anchor index.
	// Some indexes have multiple hashes as hashes cascade.  Other
	// indexes are skipped, as they summarized by values in sub
	// merkle trees on the way to the anchor index.
	//
	// This for loop adds the hashes leading up to the highest
	// sub Merkle Tree between the element index and the anchor index
	for idx := r.StartIndex; idx <= r.EndIndex; { // Range over all the elements
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
			r.Anchor = combineHashes(lHash, rHash) // We don't have to calculate the MDRoot, but it
			if stay {                              //   helps debugging.  Check if still in column
				r.Entries = append(r.Entries, &ReceiptEntry{Hash: lHash, Right: false}) // If so, combine from left
			} else { //                                                     Otherwise
				r.Entries = append(r.Entries, &ReceiptEntry{Hash: rHash, Right: true}) //  combine from right
			}
			stay = true // By default assume a stay in the column
			height++    // and increment the height.
		}
	}

	// At this point, we have reached the highest sub Merkle Tree root.
	// All we have to do is calculate the set of intermediate hashes that
	// will be combined with the current state of the receipt to match
	// the Merkle Dag Root at the Anchor Index

	if r.EndIndex == 0 { // Special case the first entry in a Merkle Tree
		r.Anchor = r.Start // whose Merkle Dag Root is just the first element
		return nil         // added to the merkle tree
	}

	stay = false // Indicate no elements for the first index have been added
	state := anchorState

	var intermediateHash, lastIH []byte // The intermediateHash tracks the combining of hashes as we go. The
	for i, v := range state.Pending {   // last hash computed is the last intermediate Hash used in an anchor
		if v == nil { //                  Skip in Pending until a value is found
			continue
		}
		if intermediateHash == nil { //   If no computations have been started,
			intermediateHash = copyHash(v) //   just move the value from pending over
			if height-1 == int64(i) {      // If height is just above entry in pending
				stay = true //                consider processing to continue
			}
			continue
		}
		lastIH = copyHash(intermediateHash)                   // compute a new intermediate hash
		intermediateHash = combineHashes(v, intermediateHash) // Combine Pending with intermediate
		if int64(i) < height-1 {                              // If not to the proof height, skip
			continue //                                                                 adding to the receipt
		}
		if stay { //                                                     If in the same column
			if int64(i) >= height { //                                    And the proof is at this hight or higher
				r.Entries = append(r.Entries, &ReceiptEntry{Hash: v, Right: false}) // Add to the receipt
			}
			continue
		}
		r.Entries = append(r.Entries, &ReceiptEntry{Hash: lastIH, Right: true}) // First time in this column, so add to receipt
		stay = true                                                             // Indicate processing the same column now.

	}
	r.Anchor = intermediateHash // The Merkle Dag Root is the last intermediate Hash produced.

	return nil
}
