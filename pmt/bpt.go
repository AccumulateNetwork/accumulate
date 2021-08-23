package pmt

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"

	"github.com/AccumulateNetwork/SMT/common"
)

// BPT
// Binary Patricia Tree.
// Two types of Entry in the Tree:
//    Node - a node in a binary tree that ends in Values (Left and Right)
//    Value - a key / value pair where the key is a ChainID and the value
//            is the hash of the state of the chain
// The BPT can be updated many times, then updated in batch (which reduces
// the hashes that have to be performed to update the summary hash)
type BPT struct {
	Root      *Node            // The root of the Patricia Tree, holding the summary hash for the Patricia Tree
	DirtyMap  map[uint64]*Node // Map of dirty nodes.
	MaxHeight int              // Highest height of any node in the BPT
	MaxNodeID uint64           // Maximum node id assigned to any node
	power     int              // Power
	mask      int              // Mask used to detect Byte Block boundaries
	manager   *Manager         // Pointer to the manager for access to the database
}

// Equal
// Used to do some testing
func (b *BPT) Equal(b2 *BPT) (equal bool) {
	defer func() {
		if err := recover(); err != nil {
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
	data = append(data, byte(b.MaxHeight))
	data = append(data, common.Uint64Bytes(b.MaxNodeID)...)
	data = append(data, byte(b.power>>8), byte(b.power))
	data = append(data, byte(b.mask>>8), byte(b.mask))
	data = append(data, b.Root.Marshal()...)
	return data
}

// UnMarshal
// Load the BPT in support of initialization from disk.  Note that
// an existing BPT will be over written completely.
func (b *BPT) UnMarshal(data []byte) (newData []byte) {
	b.DirtyMap = make(map[uint64]*Node)
	b.MaxHeight, data = int(data[0]), data[1:]
	b.MaxNodeID, data = common.BytesUint64(data)
	b.power, data = int(data[0])<<8+int(data[1]), data[2:]
	b.mask, data = int(data[0])<<8+int(data[1]), data[2:]
	data = b.Root.UnMarshal(data)
	return data
}

// NewNode
// Allocate a new Node for use with this BPT.  Note that various bookkeeping
// tasks are performed for the caller.
func (b *BPT) NewNode(parent *Node) (node *Node) {
	node = new(Node)                // Create the node
	node.Parent = parent            // Set the Parent
	node.Height = parent.Height + 1 // Make the height 1 greater than the Parent
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
// Sort if a node is in the dirty tracking. Allows batching updates for greater
// efficiency.
func (b *BPT) IsDirty(node *Node) bool { // Sort if node is in our Dirty Map
	_, ok := b.DirtyMap[node.GetID()] //     do the check
	return ok                         //     return result
}

// Clean
// Take a node out of the dirty tracking.  Don't care if it is or isn't dirty
func (b *BPT) Clean(node *Node) {
	if node == nil { //                   That said, we do care if it is nil
		return //                         If nil, nothing to do.  The root has a nil Parent.
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
	b.Clean(node.Parent)            //    Note if we handle the child, we will handle the
	//                                     Parent.  So take it out of tracking.
}

// GetDirtyList
// Convert the map to a list (must work down from the highest
// heights to the root (to keep from stomping on hashing orders; all hashes
// at the same height are independent of each other, but must be computed
// before we handle the next lowest height, and so forth.
func (b *BPT) GetDirtyList() (list []*Node) {
	for _, v := range b.DirtyMap { //             Run through the Map
		list = append(list, v) //                 Add the nodes to the list
	}
	sort.Slice(list, func(i, j int) bool { //     Now sort by height; maps randomize order
		return list[i].Height > list[j].Height // Sort by height, as said before
	})
	return list //                                Return sorted list
}

var dcnt = 0

// insertAtNode
// A recursive routine that pushes collisions towards the leaves of the
// binary patricia tree until the keys don't match any more.  Note that
// this tree cannot handle duplicate keys, but that is an assumption of
// patricia trees anyway
//
// Inputs:
// BIdx -- byte index into the key
// bit  -- index to the bit
// node -- the node in the BPT where the value (key, hash) is being inserted
// key  -- The key in the BPT which determines were in the BPT the hash goes
// hash -- The current value of the key, as tracked by the BPT
func (b *BPT) insertAtNode(BIdx, bit byte, node *Node, key, hash [32]byte) {

	if BIdx == 0 && bit == 1 {
		//fmt.Printf("key %x\n", key)
		dcnt = 0
	}
	step := func() { //  In order to reduce redundant code, we step with a
		bit <<= 1     // local function.         Inlining might provide some
		if bit == 0 { //                         performance.  What we are doing is shifting the
			bit = 1 //                           bit test up on each level of the merkle tree.  If the bit
			BIdx++  //                           shifts out of a BIdx, we increment the BIdx and start over
		}
	}

	Insert := func(e *Entry) { //                                    Again, to avoid redundant code, Left and Right
		switch { //                                                  processing is done once here.
		case *e == nil: //                                           Sort if the Left/Right is nil.
			v := b.NewValue(key, hash) //                            If it is, we can put the value here
			*e = v                     //                            so just do so.
			b.Dirty(node)              //                            And changing the value of a node makes it dirty
			return                     //                            we are done.
		case (*e).T() == TNode: //                                   If the entry isn't nil, check if it is a Node
			step()                                             //    If it is a node, then try and insert it on that node
			b.insertAtNode(BIdx, bit, (*e).(*Node), key, hash) //    Recurse up the tree
		default: //                                                  If not a node, not nil, it is a value.
			v := (*e).(*Value)                 //                    A collision. Get the value that got here first
			if bytes.Equal(key[:], v.Key[:]) { //                    If this value is the same as we are inserting
				if !bytes.Equal((*e).(*Value).Hash[:], hash[:]) { // Make sure this is really a change, i.e.
					(*e).(*Value).Hash = hash //                     the new hash is really different.  If it is
					b.Dirty(node)             //                     mark the node as dirty
				}
				return //                                            Changed or not, we are done.
			} //                                                     The idea is to create a node, to replace the value
			nn := b.NewNode(node)                        //          that was here, and the old value and the new value
			*e = nn                                      //          and insert them at one height higher.
			step()                                       //          This means we walk down the bits of both values
			nn.BBKey = GetBBKey(BIdx, key)               //          Record the nn.BBKey
			b.insertAtNode(BIdx, bit, nn, key, hash)     //          until they diverge.
			b.insertAtNode(BIdx, bit, nn, v.Key, v.Hash) //          Because these are chainIDs, while they could be
		} //                                                         mined to attack our BPT, we don't much care; it will
	} //                                                             cost the attackers more than the protocol

	if node.Left != nil && node.Left.T() == TNotLoaded ||
		node.Right != nil && node.Right.T() == TNotLoaded {
		node.BBKey = GetBBKey(BIdx, key)
		n := b.manager.LoadNode(node)
		node.Left = n.Left
		node.Right = n.Right
	}

	if bit&key[BIdx] == 0 { //      Note that this is the code that calls the Inline function Insert, and Insert
		Insert(&node.Left) //       in turn calls step.  We check the bit on the given BIdx. 0 goes Left
	} else { //                     and
		Insert(&node.Right) //      1 goes Right
	}
}

// Insert
// Starts the search of the BPT for the location of the key in the BPT
func (b *BPT) Insert(key, hash [32]byte) { //          The location of a value is determined by the key, and the value
	b.insertAtNode(0, 1, b.Root, key, hash) //         in that location is the hash.  We start at byte 0, lowest
} //                                                   significant bit. (which is masked with a 1)

// GetHash
// Makes the code just a bit more simple.  Checks for nils
func GetHash(e Entry) []byte {
	if e == nil { //              Sort for nil, return nil if e is nil.
		return nil
	}
	return e.GetHash() //         Otherwise, call the function to return the Hash for the entry.
}

// Update the Patricia Tree hashes with the values from the
// updates since the last update, and return the root hash
func (b *BPT) Update() {
	for len(b.DirtyMap) > 0 { //                           While the DirtyMap has nodes to process
		dirtyList := b.GetDirtyList() //                   Get the Dirty List. Note sorted by height, High to low

		h := dirtyList[0].Height      //                   Get current height so we do one pass at one height at a time.
		for _, n := range dirtyList { //                   go through the list, and add parents to the dirty map
			if n.Height != h { //                          Note when the height is done,
				break //                                     bap out
			} //
			if h&b.mask == 0 && b.manager != nil { //      Sort and see if at the root node for a byte block
				b.manager.FlushNode(n) //                  If so, flush the byte block; it has already been updated
			} //
			L := GetHash(n.Left)  //                       Get the Left Branch
			R := GetHash(n.Right) //                       Get the Right Branch
			switch {              //                       Sort four conditions:
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
			b.Dirty(n.Parent) //                           The Parent is dirty cause it must consider this new state
		}
	}
}

// New BPT
// Allocate a new BPT and set up the structures required to get to work with
// Binary Patricia Trees.
func NewBPT() *BPT {
	b := new(BPT)                       // Get a Binary Patrica Tree
	b.power = 8                         // using 4 bits to persist BPTs to disk
	b.mask = b.power - 1                // Take the bits to the power of 2 -1
	b.Root = new(Node)                  // Allocate summary node (contributes nothing to BPT summary Hash
	b.Root.Height = 0                   // Before the next level
	b.DirtyMap = make(map[uint64]*Node) // Allocate the Dirty Map, because batching updates is
	return b                            // a pretty powerful way to process Patricia Trees
}

// MarshalByteBlock
// Given the node leading into a byte block, marshal all the nodes within the
// block.  A borderNode is a node that completes a byte boundry.  So consider
// a theoretical key 03e706b93d2e515c6eff056ee481eb92f9e790277db91eb748b3cc5b46dfe8ca
// The first byte is 03, second is a7, third is 06 etc.
//
// The node in block 03 that completes e7 is the board node.  The Left path
// would begin the path to the theoretical key (a bit zero).
func (b *BPT) MarshalByteBlock(borderNode *Node) (data []byte) {
	if borderNode.Height&b.mask != 0 { //                Must be a boarder node
		panic("cannot call MarshalByteBlock on non-boarder nodes") //     and the code should not call this routine
	} //
	data = b.MarshalEntry(borderNode.Left, data)  //                      Marshal the Byte Block to the Left
	data = b.MarshalEntry(borderNode.Right, data) //                      Marshal the Byte Block to the Right
	return data
}

// MarshalEntry
// Recursive routine that marshals a byte block starting from an entry on
// the Left or on the Right.  Calling MarshalEntry from the Left only
// marshals half the node space of a byte block.  Have to call MarshalEntry
// from the Right to complete coverage.
func (b *BPT) MarshalEntry(entry Entry, data []byte) []byte { //

	switch {
	case entry == nil: //                                      Sort if nil
		data = append(data, TNil) //                           Mark as nil,
		return data               //                           We are done
	case entry.T() == TValue: //                               Sort if Value
		data = append(data, TValue)             //             Tag Left as a value
		data = append(data, entry.Marshal()...) //             And marshal the value
		return data                             //             Done
	case entry.T() == TNode && //                              Sort if TNode
		entry.(*Node).Height&b.mask == 0: //                   See if entry is going into
		data = append(data, TNode)              //             Mark as going into a node
		data = append(data, entry.Marshal()...) //             Put the fields into the slice
		data = append(data, TNotLoaded)         //             Left is going into next Byte Block
		data = append(data, TNotLoaded)         //             Right is going into next Byte Block
		return data                             //             Return the data
	case entry.T() == TNotLoaded: //                           Sort if node isn't loaded
		data = append(data, TNotLoaded)
	default: //                                                In this case, we have a node to marshal
		data = append(data, TNode)                       //    Mark as going into a node
		data = append(data, entry.Marshal()...)          //    Put the fields into the slice
		data = b.MarshalEntry(entry.(*Node).Left, data)  //    Marshal Left
		data = b.MarshalEntry(entry.(*Node).Right, data) //    Marshal Right
	}
	return data
}

// UnMarshalByteBlock
//
func (b *BPT) UnMarshalByteBlock(borderNode *Node, data []byte) []byte {
	if borderNode.Height&b.mask != 0 {
		panic("cannot call UnMarshalByteBlock on non-boarder nodes")
	}
	borderNode.Left, data = b.UnMarshalEntry(borderNode, data)
	borderNode.Right, data = b.UnMarshalEntry(borderNode, data)
	return data
}

func (b *BPT) UnMarshalEntry(parent *Node, data []byte) (Entry, []byte) { //
	nodeType, data := data[0], data[1:] //      Pull the node type out of the slice
	switch nodeType {                   //      Based on node time, use proper unmarshal code
	case TNil: //                               If a nil, easy
		return nil, data //                     return the data pointer (sans the type)
	case TValue: //                             If a value
		v := new(Value)          //             unmarshal the value
		data = v.UnMarshal(data) //
		return v, data           //             Return the value object and updated data slice
	case TNotLoaded: //                         If not loaded
		if parent.Height&b.mask != 0 {
			panic(fmt.Sprintf("writing a TNotLoaded node on a non-boundary node"))
		}
		return new(NotLoaded), data //          Create the NotLoaded stub and updated pointer
	case TNode: //                              If a Node
		n := new(Node)                            // Allocate a new node
		n.Parent = parent                         // Set the Parent
		data = n.UnMarshal(data)                  // Get the Node
		n.Left, data = b.UnMarshalEntry(n, data)  // Populate Left
		n.Right, data = b.UnMarshalEntry(n, data) // Populate Right
		return n, data
	}
	panic("failure to decode the ByteBlock")
}
