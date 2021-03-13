package smt

import "crypto/sha256"

type ReceiptNode struct {
	Right bool // The given Hash will be on the Right (right==true) or on the left (right==false)
	Hash  Hash // hash to be combined at the next level
}

type MDReceipt struct {
	EntryHash Hash           // Entry Hash of the data subject to the MDReceipt
	Nodes     []*ReceiptNode // Path through the data collected by the MerkleDag
	MDRoot    Hash           // Merkle DAG root from the Accumulator network.
	// We likely want a struct here provided by the underlying blockchain where we are recording
	// the MDRoots for the Accumulator
}

// BuildMDReceipt
// Building a receipt is a bit more complex than just computing the Merkle DAG.  We look at the stream of
// hashes as we build the MerkleState, and detect when the Hash for which we need a receipt shows up.  Then we
// track it through the intermediate hashes, tracking if it is combined from the right or from the left.
// Then when we have to calculate the Merkle DAG root, we do one more pass through the combining of the
// trailing hashes to finish off the receipt.
func (mdr *MDReceipt) BuildMDReceipt(MerkleDag MerkleState, data Hash) {
	mdr.Nodes = mdr.Nodes[:0] // Throw away any old paths
	mdr.EntryHash = data      // The Data for which this is a Receipt
	md := []*Hash{nil}        // The intermediate hashes used to compute the Merkle DAG root
	right := true             // We assume we will be combining from the right
	idx := -1                 // idx of -1 means not yet found the hash for which we want a receipt in the hash stream

DataLoop: // Loop through the data behind the Merkle DAG and rebuild the MerkleState state
	for _, h := range MerkleDag.HashList {
		right = true // Generally our "place" is in md[], so the hash we combine with will be on the right
		// Always make sure md ends with a nil; limits corner cases
		if md[len(md)-1] != nil { // Make sure md still ends in a nil
			md = append(md, nil)
		}
		// Look for the data that we are computing a receipt for
		// If the data is duplicated, we only pay attention to the first instance, but every
		// hash should be unique.  We could take an index of an entry to allow proof of any
		// of a set of duplicate hashes, but we don't do that here.
		if idx < 0 && h == data {
			idx = 0
			right = false // Here we found our hash, so we will be (maybe) combining with a hash on the left
		}
		// Then add this data to the Merkle DAG we are creating
		for i, v := range md {
			if v == nil { // If v is nil, then we move h to md[i].
				md[i] = h.CopyAndPoint()
				continue DataLoop // When we move a hash to md[], then we are done and need to get another h
			}
			if i == idx { // If we are on the path, and we are going to combine, then record
				rn := new(ReceiptNode)
				mdr.Nodes = append(mdr.Nodes, rn)
				rn.Right = right
				if right { // If our hash is on the right, we need to record the hash on the left
					rn.Hash = h
				} else { // If our hash is on the left, we need to record the hash on the right
					rn.Hash = *v
				}
				right = false // Regardless, our hash is now in h and will later combine with a hash on the left
				idx++         // And our hash will "carry" to the next slot in md[]
			}
			h = v.Combine(MerkleDag.HashFunction, h) // Combine v (left) & hash (right) => new combined hash
			md[i] = nil                              // Now that we have combined v and hash, this spot is now empty, so clear it.
		}
	}
	// At this point we have a (possibly) partial merkle tree.
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
	//
	//  What is interesting is that the []md state looks like before we create the Merkle DAG root is:
	//     __
	//     5+6
	//     12+34
	//     __
	// So the last two nodes in the receipt will either be
	//      left 1234
	// if the proof is either for 5 or 6, or
	//      right 56
	// if the proof is for 1, 2, 3, or 4
	//
	if idx == -1 {
		mdr.Nodes = mdr.Nodes[:0]
		return
	}
	var mdRoot *Hash // mdRoot is the merkle DAG root we are building.
	right = true

	// We close the Merkle DAG
	for i, v := range md {
		if mdRoot == nil { // If we have not found a hash yet, try and pick one up here.
			if i == idx {
				if v == nil { // This should never happen.
					panic("found our hash level, but no hash was found.")
				}
				right = false // Now our hash is on the right, will be combined with a hash on the left, or none at all.
				idx++
			}
			if v != nil {
				mdRoot = new(Hash)
				copy(mdRoot[:], v[:]) // Pick up this hash, and look for the next
			}
			continue
		}
		if v != nil { // If MDRoot isn't nil and v isn't nil, we combine them.
			if i == idx {
				rn := new(ReceiptNode)
				mdr.Nodes = append(mdr.Nodes, rn)
				rn.Right = right
				if right {
					copy(rn.Hash[:], mdRoot[:])
				} else {
					copy(rn.Hash[:], v[:])
				}
				right = false
				idx++
			}
			// v is on the left, MDRoot candidate is on the right, for a new MDRoot
			mdRoot_ := v.Combine(MerkleDag.HashFunction, *mdRoot) // A pointer to the result is needed, so assign the
			mdRoot = &mdRoot_                                     // result to the variable mdRoot_ and use its address
			mdr.MDRoot = *mdRoot                                  // The last one is the one we want
		}
	}
	return
}

// Validate
// Run down the Merkle DAG and prove that this receipt self validates
func (mdr *MDReceipt) Validate() bool {
	f := func(data []byte) Hash { return sha256.Sum256(data) }
	hash := mdr.EntryHash
	for _, n := range mdr.Nodes {
		if n.Right {
			hash = hash.Combine(f, n.Hash)
		} else {
			hash = n.Hash.Combine(f, hash)
		}
	}
	return hash == mdr.MDRoot
}
