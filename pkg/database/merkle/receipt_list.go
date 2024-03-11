// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

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

// Validate
// Take a receipt and validate that the element hash progresses to the
// Merkle Dag Root hash (MDRoot) in the receipt
func (r *ReceiptList) Validate(opts *ValidateOptions) bool {
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
		MS.AddEntry(append([]byte{}, h...)) // Add each element to the MS
	} //                                      Once all elements are added, compute
	anchor := MS.Anchor() //                 the anchor at this point.
	if len(anchor) == 0 { //               If an anchor can't be produced, this
		return false //                         receiptList fails. (shouldn't happen)
	}

	lastElement := r.Elements[len(r.Elements)-1]    // Make sure the last Element is the
	if !bytes.Equal(lastElement, r.Receipt.Start) { // starting hash of the Receipt
		return false
	}

	if !bytes.Equal(r.Receipt.Anchor, anchor) { // The Anchor must match the receipt
		return false
	}

	if !r.Receipt.Validate(opts) { //             The Receipt must be valid
		return false
	}

	if r.ContinuedReceipt != nil { //                           If the ReceiptList is anchored,
		if !bytes.Equal(anchor, r.ContinuedReceipt.Start) { //  Make sure it properly anchors this
			return false //                                     ReceiptList
		}
		if !r.ContinuedReceipt.Validate(opts) { //                  If it does, it still has to be valid
			return false //
		} //
	}

	return true // all is good.
}

// Included
// Tests an entry for inclusion in the given ReceiptList
// Note that while a ReceiptList proves inclusion in a Merkle Tree, and the fact
// that the list of elements proceed in order up to and including the anchor point,
// the ReceiptList does not necessarily prove the indices of the elements in the
// Merkle Tree.  This could be solved by salting Receipts with the index of the
// hash at the anchor point.
func (r *ReceiptList) Included(entry []byte) bool {
	for _, e := range r.Elements { // Every entry in Elements is included
		if bytes.Equal(e, entry) { // in the ReceiptList proof.
			return true
		}
	}
	return false
}

// NewReceiptList
// Return a new ReceiptList with at least a MerkleState initialized
func NewReceiptList() *ReceiptList {
	r := new(ReceiptList)
	r.MerkleState = new(State)
	return r
}

// GetReceiptList
// Given a merkle tree with a start point and an end point, create a ReceiptList for
// all the elements from the start hash to the end hash, inclusive.
func GetReceiptList(manager *Chain, Start int64, End int64) (r *ReceiptList, err error) {
	if Start > End { // Start must be before the end
		return nil, fmt.Errorf("start %d and end %d is invalid for ReceiptList", Start, End)
	}

	// Make sure the end is in the range of the Merkle Tree
	ms, err := manager.Head().Get()
	if err != nil {
		return nil, err
	}
	if ms.Count <= End {
		return nil, fmt.Errorf("end %d is out of range for SMT length %d", End, ms.Count)
	}

	// Allocate the ReceiptList, add all the elements to the ReceiptList
	r = NewReceiptList()
	for i := Start; i <= End; i++ { // Get all the elements for the list
		h, err := manager.Entry(i)
		if err != nil {
			return nil, err
		}
		r.Elements = append(r.Elements, copyHash(h))
	}

	r.MerkleState, err = manager.StateAt(Start - 1)
	if err != nil {
		return nil, err
	}
	lastElement := append([]byte{}, r.Elements[len(r.Elements)-1]...)
	r.Receipt, err = getReceipt(manager, lastElement, lastElement)
	if err != nil {
		return nil, err
	}

	return r, nil
}
