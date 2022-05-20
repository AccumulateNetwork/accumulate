package types

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
)

func doSha256(b ...[]byte) []byte {
	digest := sha256.New()
	for _, b := range b {
		_, _ = digest.Write(b)
	}
	return digest.Sum(nil)
}

func copyBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

// String
// Convert the receipt to a string
func (r *Receipt) String() string {
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("\nStart      %x\n", r.Start))       // Start of proof
	b.WriteString(fmt.Sprintf("StartIndex %d\n", r.StartIndex))    // Start of proof
	b.WriteString(fmt.Sprintf("Anchor       %x\n", r.Anchor))      // Anchor point in the Merkle Tree
	b.WriteString(fmt.Sprintf("AnchorIndex  %d\n", r.AnchorIndex)) // Anchor point in the Merkle Tree
	b.WriteString(fmt.Sprintf("Result       %x\n", r.Result))      // Result result of evaluating the receipt path
	working := r.Start                                             // Calculate the receipt path; for debugging print the
	for i, v := range r.Entries {                                  // intermediate hashes
		r := "L"
		if v.Right {
			r = "R"
			working = doSha256(working, v.Hash)
		} else {
			working = doSha256(v.Hash, working)
		}
		b.WriteString(fmt.Sprintf(" %10d Apply %s %x working: %x \n", i, r, v.Hash, working))
	}
	return b.String()
}

// Validate
// Take a receipt and validate that the Start hash progresses to the
// Merkle Dag Root hash (Result) in the receipt
func (r *Receipt) Validate() bool {
	Result := r.Start // To begin with, we start with the object as the Result
	// Now apply all the path hashes to the Result
	for _, node := range r.Entries {
		// Need a [32]byte to slice
		hash := node.Hash
		if node.Right {
			// If this hash comes from the right, apply it that way
			Result = doSha256(Result, hash)
			continue
		}
		// If this hash comes from the left, apply it that way
		Result = doSha256(hash, Result)
	}
	// In the end, Result should be the same hash the receipt expects.
	return bytes.Equal(Result, r.Result)
}

// Combine
// Take a 2nd receipt, attach it to a root receipt, and return the resulting
// receipt.  The idea is that if this receipt is anchored into another chain,
// Then we can create a receipt that proves the start in this receipt all
// the way down to an anchor in the root receipt.
// Note that both this receipt and the root receipt are expected to be good.
func (r *Receipt) Combine(rm *Receipt) (*Receipt, error) {
	if !bytes.Equal(r.Result, rm.Start) {
		return nil, fmt.Errorf("receipts cannot be combined. "+
			"anchor %x doesn't match root merkle tree %x", r.Anchor, rm.Start)
	}
	nr := r.Copy()                 // Make a copy of the first Receipt
	nr.Result = rm.Result          // The Result will be the one from the appended receipt
	for _, n := range rm.Entries { // Make a copy and append the Entries of the appended receipt
		nr.Entries = append(nr.Entries, *n.Copy())
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

type GetIntermediateFunc func(element, height uint64) (l, r []byte, err error)

func (r *Receipt) Build(getIntermediate GetIntermediateFunc, pending [][32]byte) error {
	height := uint64(1) // Start the height at 1, because the element isn't part
	r.Result = r.Start  // of the nodes collected.  To begin with, the element is the Merkle Dag Root
	stay := true        // stay represents the fact that the proof is already in this column

	// The path from a intermediateHash added to the merkle tree to the anchor
	// starts at the element index and goes to the anchor index.
	// Some indexes have multiple hashes as hashes cascade.  Other
	// indexes are skipped, as they summarized by values in sub
	// merkle trees on the way to the anchor index.
	//
	// This for loop adds the hashes leading up to the highest
	// sub Merkle Tree between the element index and the anchor index
	for idx := r.StartIndex; idx <= r.AnchorIndex; { // Range over all the elements
		if idx&1 == 0 { // Handle the even cases. Merkle Trees add elements at 0, and combine elements at odd numbers
			idx++        // No point in handling the first intermediateHash, so move to the next column
			stay = false // The proof is lagging in the previous column.
		} else { // The Odd cases hold summary hashes
			lHash, rHash, err := getIntermediate(idx, height) // Get the previous hight left/right hashes
			if err != nil {                                   // Error means end of the column has been reached
				next := uint64(math.Pow(2, float64(height-1))) //            Move to the next column 2^(height-1) columns
				idx += next                                    //
				stay = false                                   //            Changing columns
				continue
			}
			r.Result = doSha256(lHash, rHash) // We don't have to calculate the Result, but it
			if stay {                         //   helps debugging.  Check if still in column
				r.Entries = append(r.Entries, ReceiptEntry{Hash: lHash, Right: false}) // If so, combine from left
			} else { //                                                     Otherwise
				r.Entries = append(r.Entries, ReceiptEntry{Hash: rHash, Right: true}) //  combine from right
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
		r.Result = r.Start // whose Merkle Dag Root is just the first element
		return nil         // added to the merkle tree
	}

	stay = false // Indicate no elements for the first index have been added

	var intermediateHash, lastIH []byte // The intermediateHash tracks the combining of hashes as we go. The
	for i, v := range pending {         // last hash computed is the last intermediate Hash used in an anchor
		if v == ([32]byte{}) { //                  Skip in Pending until a value is found
			continue
		}
		v := v                       // See docs/developer/rangevarref.md
		if intermediateHash == nil { //   If no computations have been started,
			intermediateHash = v[:]    //   just move the value from pending over
			if height-1 == uint64(i) { // If height is just above entry in pending
				stay = true //                consider processing to continue
			}
			continue
		}
		lastIH = copyBytes(intermediateHash)                // compute a new intermediate hash
		intermediateHash = doSha256(v[:], intermediateHash) // Combine Pending with intermediate
		if uint64(i) < height-1 {                           // If not to the proof height, skip
			continue //                                                                 adding to the receipt
		}
		if stay { //                                                     If in the same column
			if uint64(i) >= height { //                                    And the proof is at this hight or higher
				r.Entries = append(r.Entries, ReceiptEntry{Hash: v[:], Right: false}) // Add to the receipt
			}
			continue
		}
		r.Entries = append(r.Entries, ReceiptEntry{Hash: lastIH, Right: true}) // First time in this column, so add to receipt
		stay = true                                                            // Indicate processing the same column now.

	}
	r.Result = intermediateHash // The Merkle Dag Root is the last intermediate Hash produced.

	return nil
}
