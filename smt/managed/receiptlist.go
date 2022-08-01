package managed

import (
	"bytes"
	"fmt"
)

// ReceiptList
// A ReceiptList is a generic proof that can be used to prove each element
// in the elements list are part of the merkle tree, and exist in the order
// given.  The last element of ReceiptList.Elements is provably part of the
// Overall Merkle Tree, as proven by the Receipt.
//
// If anchored into another Second Merkle Tree, this proof can be extended
// by combining receipt built off the Second Merkle Tree with the receipt
// given in the ReceiptList
type ReceiptList struct {
	Element     []byte       // Element we wish to prove to be in the list
	MerkleState *MerkleState // Starting Merkle State
	Elements    [][]byte     // Element hashes.  Last entry is the anchor
	Receipt     *Receipt     // Receipt for the last hash in Elements (the anchor)
}

// Validate
// Take a receipt and validate that the element hash progresses to the
// Merkle Dag Root hash (MDRoot) in the receipt
func (r *ReceiptList) Validate() bool {
	// Make sure the ReceiptList isn't empty.  Avoids pointer and nil exceptions
	if r.MerkleState == nil ||
		r.Receipt == nil ||
		r.Elements == nil || len(r.Elements) == 0 {
		return false
	}

	MS := r.MerkleState.Copy()     //         Get the Merkle State, and walk it
	for _, h := range r.Elements { //           to the end of the list.
		if h == nil || len(h) != 32 { //      Make sure every element in Elements
			return false //                     is a proper hash and not nil
		} //
		MS.AddToMerkleTree(h) //              Add each element to the MS
	} //                                      Once all elements are added, compute
	anchor := MS.GetMDRoot()               //   the anchor at this point.
	if anchor == nil || len(anchor) == 0 { // If an anchor can't be produced, this
		return false //                         receipt fails.
	}

	if !bytes.Equal(r.Elements[0],r.Receipt.Start) {
		return false
	}

	if !r.Receipt.Validate() { //                 The Receipt must be valid
		return false
	}

	// The receipt doesn't match the ReceiptList.
	return true
}

// NewReceiptList
// Return a new ReceiptList with at least a MerkleState initialized
func NewReceiptList() *ReceiptList {
	r := new(ReceiptList)
	r.MerkleState = new(MerkleState)
	return r
}

// GetReceiptList
// Given a merkle tree with a start point and an end point, create a ReceiptList for
// all the elements from the start hash to the end hash, inclusive.
func GetReceiptList(manager *MerkleManager, element []byte, Start int64, End int64) (r *ReceiptList, err error) {
	if Start > End { // Start must be before the end
		return nil, fmt.Errorf("start %d and end %d is invalid for ReceiptList", Start, End)
	}

	// Make sure the end is in the range of the Merkle Tree
	ms, err := manager.Head().Get()
	if ms.Count <= End {
		return nil, fmt.Errorf("end %d is out of range for SMT length %d", End, ms.Count)
	}

	// Make sure the element is the right length for a hash
	if element == nil || len(element) != 32 {
		lenElement := len(element)
		truncate := ""
		if lenElement > 32 {
			element = element[:32]
			truncate = fmt.Sprintf("... Len(element)=%d", lenElement)
		}
		return nil, fmt.Errorf("The element provided is not a valid hash %x%s", element, truncate)
	}

	// Allocate the ReceiptList, add all the elements to the ReceiptList
	r = NewReceiptList()
	r.Element = element
	found := false
	for i := Start; i <= End; i++ { // Get all the elements for the list
		h, err := manager.Get(i)
		if err != nil {
			return nil, err
		}
		if bytes.Equal(element, h) {
			found = true
		}
		r.Elements = append(r.Elements, h)
	}
	if !found {
		return nil, fmt.Errorf("%x is not found in the elements of the ReceiptList", element)
	}

	r.MerkleState, err = manager.GetAnyState(Start - 1)
	if err != nil {
		return nil, err
	}
	r.Receipt, err = GetReceipt(manager, r.Elements[0], r.Elements[len(r.Elements)-1])
	if err != nil {
		return nil, err
	}

	return r, nil
}
