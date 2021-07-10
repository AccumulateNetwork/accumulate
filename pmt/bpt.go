package pmt

import (
	"bytes"
	"crypto/sha256"
	"sort"

	"github.com/AccumulateNetwork/SMT/storage"
)

// BPT
// Binary Patricia Tree.
// Two types of Entry in the Tree:
//    Node - a node in a binary tree that ends in Values (left and right)
//    Value - a key / value pair where the key is a ChainID and the value
//            is the hash of the state of the chain
// The BPT can be updated many times, then updated in batch (which reduces
// the hashes that have to be performed to update the summary hash)
type BPT struct {
	Root      Entry           // The root of the Patricia Tree, holding the summary hash for the Patricia Tree
	DirtyMap  map[int64]*Node // Map of dirty nodes.
	MaxHeight byte            // Highest height of any node in the BPT
	MaxNodeID int64           // Maximum node id assigned to any node
}

// Equal
// Used to do some testing
func (b *BPT) Equal(b2 *BPT) (equal bool) {
	defer func() {
		if err := recover(); err != 0 {
			equal = false
		}
	}()

	if !b.Root.Equal(b2.Root) {
		return false
	}
	if b.MaxHeight != b2.MaxHeight {
		return false
	}
	if b.MaxNodeID != b2.MaxNodeID {
		return false
	}
	return true
}

// Marshal
// Must have the MaxNodeID at the very least to be able to add nodes
// to the BPT
func (b *BPT) Marshal() (data []byte) {
	data = append(data, storage.Int64Bytes(int64(b.MaxHeight))...)
	data = append(data, storage.Int64Bytes(int64(b.MaxNodeID))...)
	return data
}

// UnMarshal
// Reload the BPT
func (b *BPT) UnMarshal(data []byte) (newData []byte) {
	b.MaxHeight, data = data[0], data[1:]
	b.MaxNodeID, data = storage.BytesInt64(data)
	return data
}

// NewNode
// Allocate a new Node for use with this BPT.  Note that various bookkeeping
// tasks are performed for the caller.
func (b *BPT) NewNode(parent *Node) (node *Node) {
	node = new(Node)                // Create the node
	node.parent = parent            // Set the parent
	node.Height = parent.Height + 1 // Make the height 1 greater than the parent
	if node.Height > b.MaxHeight {  // If the height is the biggest we have seen
		b.MaxHeight = node.Height //     then keep it as the new max height
	}
	b.MaxNodeID++         //           Increment the Node ID (no zero ID nodes)
	node.ID = b.MaxNodeID //           Assign the Node ID to this node
	return node           //           done
}

// NewValue
// Allocate a new Value struct and do some bookkeeping for the user
func (b *BPT) NewValue(key, hash [32]byte) (value *Value) {
	value = new(Value) //              Allocate the value
	value.Key = key    //              Set the key
	value.Hash = hash  //              Set the ChainID (which is a hash)
	return value       //              That's all that we have to do
}

// IsDirty
// Check if a node is in the dirty tracking. Allows batching updates for greater
// efficiency.
func (b *BPT) IsDirty(node *Node) bool { // Check if node is in our Dirty Map
	_, ok := b.DirtyMap[node.GetID()] //     do the check
	return ok                         //     return result
}

// Clean
// Take a node out of the dirty tracking.  Don't care if it is or isn't dirty
func (b *BPT) Clean(node *Node) {
	if node == nil { //                   That said, we do care if it is nil
		return //                         If nil, nothing to do.  The root has a nil parent.
	}
	delete(b.DirtyMap, node.GetID()) //   If not nil, delete it from the map
} //                                      Note this doesn't matter if the node
//                                        isn't in the map

// Dirty
// Add a node to the dirty tracking
func (b *BPT) Dirty(node *Node) {
	if node == nil { //                   Errors occur if nils are not removed
		return //                         done if nil
	}
	b.DirtyMap[node.GetID()] = node //    Put the node in dirty tracking map
	b.Clean(node.parent)            //    Note if we handle the child, we will handle the
	//                                     parent.  So take it out of tracking.
}

// DirtyList
// Convert the map to a list (must work down from the highest
// heights to the root (to keep from stomping on hashing orders; all hashes
// at the same height are independent of each other, but must be computed
// before we handle the next lowest height, and so forth.
func (b *BPT) DirtyList() (list []*Node) {
	for _, v := range b.DirtyMap { //             Run through the Map
		list = append(list, v) //                 Add the nodes to the list
	}
	sort.Slice(list, func(i, j int) bool { //     Now sort by height; maps randomize order
		return list[i].Height > list[j].Height // Sort by height, as said before
	})
	return list //                                Return sorted list
}

// insertAtNode
// A recursive routine that pushes collisions towards the leaves of the
// binary patricia tree until the keys don't match any more.  Note that
// this tree cannot handle duplicate keys, but that is an assumption of
// patricia trees anyway
func (b *BPT) insertAtNode(byte, bit byte, node *Node, key, hash [32]byte) {

	step := func() { //  In order to reduce redundant code, we step with a
		bit <<= 1     // local function.         Inlining might provide some
		if bit == 0 { //                         performance.  What we are doing is shifting the
			bit = 1 //                           bit test up on each level of the merkle tree.  If the bit
			byte++  //                           shifts out of a byte, we increment the byte and start over
		}
	}

	Insert := func(e *Entry) { //                        Again, to avoid redundant code, left and right
		switch { //                                      processing is done once here.
		case *e == nil: //                                        Check if the Left/Right is nil.
			v := b.NewValue(key, hash) //                          If it is, we can put the value here
			*e = v                     //                          so just do so.
			b.Dirty(node)              //                          And changing the value of a node makes it dirty
			return                     //                          we are done.
		case (*e).T(): //                                         If the entry isn't nil, check if it is a Node
			step()                                             //  If it is a node, then try and insert it on that node
			b.insertAtNode(byte, bit, (*e).(*Node), key, hash) //  Recurse up the tree
		default: //                                               If not a node, not nil, it is a value.
			v := (*e).(*Value)                 //                  A collision. Get the value that got here first
			if bytes.Equal(key[:], v.Key[:]) { //                  If this value is the same as we are inserting
				(*e).(*Value).Hash = hash
				return
			} //                                                   The idea is to create a node, to replace the value
			nn := b.NewNode(node)                        //        that was here, and the old value and the new value
			*e = nn                                      //        and insert them at one height higher.
			step()                                       //        This means we walk down the bits of both values
			b.insertAtNode(byte, bit, nn, key, hash)     //        until they diverge.
			b.insertAtNode(byte, bit, nn, v.Key, v.Hash) //        Because these are chainIDs, while they could be
		} //                                                       mined to attack our BPT, we don't much care; it will
	} //                                                           cost the attackers more than the protocol

	if bit&key[byte] == 0 { //      Note that this is the code that calls the Inline function Insert, and Insert
		Insert(&node.left) //       in turn calls step.  We check the bit on the given byte. 0 goes left
	} else { //                     and
		Insert(&node.right) //      1 goes right
	}
}

// Insert
// Starts the search of the BPT for the location of the key in the BPT
func (b *BPT) Insert(key, hash [32]byte) { //          The location of a value is determined by the key, and the value
	b.insertAtNode(0, 1, b.Root.(*Node), key, hash) // in that location is the hash.  We start at byte 0, lowest
} //                                                   significant bit. (which is masked with a 1)

// GetHash
// Makes the code just a bit more simple.  Checks for nils
func GetHash(e Entry) []byte {
	if e == nil { //              Check for nil, return nil if e is nil.
		return nil
	}
	return e.GetHash() //         Otherwise, call the function to return the Hash for the entry.
}

// Update the Patricia Tree hashes with the values from the
// updates since the last update, and return the root hash
func (b *BPT) Update() [32]byte {
	for len(b.DirtyMap) > 0 { //                           While the DirtyMap has nodes to process
		dirtyList := b.DirtyList() //                      Get the Dirty List. Note sorted by height, High to low

		h := dirtyList[0].Height      //                   Get current height so we do one pass at one height at a time.
		for _, n := range dirtyList { //                   go through the list, and add parents to the dirty map
			if n.Height != h { //                          Note when the height is done,
				break //                                     bap out
			} //
			L := GetHash(n.left)  //                       Get the Left Branch
			R := GetHash(n.right) //                       Get the Right Branch
			switch {              //                       Check four conditions:
			case L != nil && R != nil: //                  If we have both L and R then combine
				n.Hash = sha256.Sum256(append(L, R...)) // Take the hash of L+R
			case L != nil: //                              The next condition is where we only have L
				copy(n.Hash[:], L) //                      Just use L.  No hash required
			case R != nil: //                              Just have R.  Again, just use R.
				copy(n.Hash[:], R) //                      No Hash Required
			default: //                                    The fourth condition never happens, and bad if it does.
				panic("dead nodes should not exist") //      This is a node without a child somewhere up the tree.
			}
			b.Clean(n)        //                           Node has been updated, so it is clean
			b.Dirty(n.parent) //                           The parent is dirty cause it must consider this new state
		}
	}
	return b.Root.(*Node).Hash //                          Summary hash in root.  Return it.
}

// New BPT
// Allocate a new BPT and set up the structures required to get to work with
// Binary Patricia Trees.
func NewBPT() *BPT {
	b := new(BPT)                      // Get a Binary Patricai Tree
	b.Root = new(Node)                 // Allocate the summary node (contributes nothing to the BPT summary Hash
	b.DirtyMap = make(map[int64]*Node) // Allocate the Dirty Map, because batching updates is
	return b                           // a pretty powerful way to process Patricia Trees
}

// MarshalByteBlock
// Given the node leading into a byte block, marshal all the nodes within the
// block.  A borderNode is a node that completes a byte boundry.  So consider
// a theoretical key 03e706b93d2e515c6eff056ee481eb92f9e790277db91eb748b3cc5b46dfe8ca
// The first byte is 03, second is a7, third is 06 etc.
//
// The node in block 03 that completes e7 is the board node.  The left path
// would begin the path to the theoretical key (a bit zero).
func MarshalByteBlock(borderNode *Node) (data []byte) {
	if borderNode.Height&7 != 0 { //                                    Must be a boarder node
		panic("cannot call MarshalByteBlock on non-boarder nodes") //   and the code should not call this routine
	} //
	data = MarshalEntry(borderNode.left, data)  //                      Marshal the Byte Block to the left
	data = MarshalEntry(borderNode.right, data) //                      Marshal the Byte Block to the right
	return data
}

// MarshalEntry
// Recursive routine that marshals a byte block starting from an entry on
// the left or on the right.  Calling MarshalEntry from the left only
// marshals half the node space of a byte block.  Have to call MarshalEntry
// from the right to complete coverage.
func MarshalEntry(entry Entry, data []byte) []byte { //
	switch {
	case entry == nil: //                           Check if nil
		data = append(data, 0) //                   Mark as nil,
		return data            //                   We are done
	case !entry.T(): //                             Check if Value
		data = append(data, 1)                  //  Tag left as a value
		data = append(data, entry.Marshal()...) //  And marshal the value
		return data                             //  Done
	case entry.(*Node).Height&0x7 == 0: //          See if entry is going into
		data = append(data, 2) //                   the next Byte Block
		return data            //                   Ignore if so and done
	default: //
		data = append(data, 3)                         //                    Mark as going into a node
		data = MarshalEntry(entry.(*Node).left, data)  // Marshal left
		data = MarshalEntry(entry.(*Node).right, data) // Marshal right
	}
	return data
}

// UnMarshalByteBlock
//
func UnMarshalByteBlock(boarderNode *Node, data []byte) []byte {
	return nil
}
