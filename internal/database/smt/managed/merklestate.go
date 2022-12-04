// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package managed

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
)

// MerkleState
// A Merkle Dag State is the state kept while building a Merkle Tree.  Except where a Merkle Tree has a clean
// power of two number of elements as leaf nodes, there will be multiple Sub Merkle Trees that make up a
// dynamic Merkle Tree. The Merkle State is the list of the roots of these sub Merkle Trees, and the
// combination of these roots provides a Directed Acyclic Graph (DAG) to all the leaves.
//
//	                                                     Merkle State
//	1  2   3  4   5  6   7  8   9 10  11 12  13 --->         13
//	 1-2    3-4    5-6    7-8   0-10  11-12     --->         --
//	    1-2-3-4       5-6-7-8    0-10-11-12     --->     0-10-11-12
//	          1-2-3-4-5-6-7-8                   --->   1-2-3-4-5-6-7-8
//
// Interestingly, the state of building such a Merkle Tree looks just like counting in binary.  And the
// higher order bits set will correspond to where the binary roots must be kept in a Merkle state.
//
//	type MerkleState struct {
//		Count    int64          // Count of hashes added to the Merkle tree
//		Pending  SparseHashList // Array of hashes that represent the left edge of the Merkle tree
//		HashList HashList       // List of Hashes in the order added to the chain
//	}
type MerkleState = record.ChainHead

type ChainType = record.ChainType

const ChainTypeUnknown = record.ChainTypeUnknown
const ChainTypeTransaction = record.ChainTypeTransaction
const ChainTypeAnchor = record.ChainTypeAnchor
const ChainTypeIndex = record.ChainTypeIndex
