// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pmt

import (
	"bytes"
	"crypto/sha256"
	"sort"
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
	RootHash  [32]byte              // Root hash of the BPT
	Root      *BptNode              // The root of the Patricia Tree, holding the summary hash for the Patricia Tree
	DirtyMap  map[[32]byte]*BptNode // Map of dirty nodes.
	MaxHeight int                   // Highest height of any node in the BPT
	Power     int                   // Power
	Mask      int                   // Mask used to detect Byte Block boundaries
	Manager   *Manager              // Pointer to the manager for access to the database
}

// GetRoot
// Get the Root node for the BPT.  This may load the BPT Root node from disk if not loaded yet.
func (b *BPT) GetRoot() (root *BptNode) {
	if b.Root == nil { //                              If we have a root node, we are good
		rootNodeKey, _ := GetNodeKey(0, [32]byte{}) // Get the root Node Key
		b.Root = new(BptNode)                       // Allocate a Root Node
		if b.Manager != nil {                       // If we have a manager, pull from the DB
			if data, err := b.Manager.DBManager.Get(kBpt.Append(rootNodeKey)); err == nil {
				b.Root.UnMarshal(data) //              Unmarshal what we get from the DB
			}
		}
		b.Root.Height = 0            //                Height of root is always zero
		b.Root.NodeKey = rootNodeKey //                The rootNodeKey is an array of zeros
	}
	return b.Root
}

// Equal
// Used to do some testing
func (b *BPT) Equal(b2 *BPT) (equal bool) {
	defer func() {
		if err := recover(); err != nil {
			equal = false
		}
	}()

	if !b.GetRoot().Equal(b2.GetRoot()) {
		return false
	}
	if b.MaxHeight != b2.MaxHeight {
		return false
	}
	return true
}

// Marshal
// Must have the MaxNodeID at the very least to be able to add nodes
// to the BPT
func (b *BPT) Marshal() (data []byte) {
	data = append(data, byte(b.MaxHeight))
	data = append(data, byte(b.Power>>8), byte(b.Power))
	data = append(data, byte(b.Mask>>8), byte(b.Mask))
	data = append(data, b.RootHash[:]...)
	return data
}

// UnMarshal
// Load the BPT in support of initialization from disk.  Note that
// an existing BPT will be over written completely.
func (b *BPT) UnMarshal(data []byte) (newData []byte) {
	b.DirtyMap = make(map[[32]byte]*BptNode)
	b.MaxHeight, data = int(data[0]), data[1:]
	b.Power, data = int(data[0])<<8+int(data[1]), data[2:]
	b.Mask, data = int(data[0])<<8+int(data[1]), data[2:]
	copy(b.RootHash[:], data[:32])
	data = data[:32]
	return data
}

// NewNode
// Allocate a new Node for use with this BPT.  Note that various bookkeeping
// tasks are performed for the caller.
func (b *BPT) NewNode(key [32]byte, parent *BptNode) (node *BptNode) {
	node = new(BptNode)                            // Create the node
	node.Parent = parent                           // Set the Parent
	node.Height = parent.Height + 1                // Make the height 1 greater than the Parent
	node.NodeKey, _ = GetNodeKey(node.Height, key) // Set the NodeKey
	if node.Height > b.MaxHeight {                 // If the height is the biggest we have seen
		b.MaxHeight = node.Height //                   then keep it as the new max height
	}
	return node //           done
}

// NewValue
// Allocate a new Value struct and do some bookkeeping for the user
func (b *BPT) NewValue(parent *BptNode, key, hash [32]byte) (value *Value) {
	value = new(Value) //              Allocate the value
	value.Key = key    //              Set the key
	value.Hash = hash  //              Set the ChainID (which is a hash)
	return value       //              That's all that we have to do
}

// IsDirty
// Sort if a node is in the dirty tracking. Allows batching updates for greater
// efficiency.
func (b *BPT) IsDirty(node *BptNode) bool { // Sort if node is in our Dirty Map
	_, ok := b.DirtyMap[node.NodeKey] //     do the check
	return ok                         //     return result
}

// Clean
// Take a node out of the dirty tracking.  Don't care if it is or isn't dirty
func (b *BPT) Clean(node *BptNode) {
	if node == nil { //                   That said, we do care if it is nil
		return //                         If nil, nothing to do.  The root has a nil Parent.
	}
	delete(b.DirtyMap, node.NodeKey) //   If not nil, delete it from the map
} //                                      Note this doesn't matter if the node
//                                        isn't in the map

// Dirty
// Add a node to the dirty tracking
func (b *BPT) Dirty(node *BptNode) {
	if node == nil { //                   Errors occur if nils are not removed
		return //                         done if nil
	}
	b.DirtyMap[node.NodeKey] = node //    Put the node in dirty tracking map
}

// GetDirtyList
// Convert the map to a list (must work down from the highest
// heights to the root (to keep from stomping on hashing orders; all hashes
// at the same height are independent of each other, but must be computed
// before we handle the next lowest height, and so forth.
func (b *BPT) GetDirtyList() (list []*BptNode) {
	for _, v := range b.DirtyMap { //             Run through the Map
		list = append(list, v) //                 Add the nodes to the list
	}
	sort.Slice(list, func(i, j int) bool { //     Now sort by height; maps randomize order
		return list[i].Height > list[j].Height // Sort by height, as said before
	})
	return list //                                Return sorted list
}

// LoadNext
// Load the next level of nodes if it is necessary.  We build Byte Blocks of
// nodes which store all the nodes and values within 8 bits of a key.  If the
// key is longer, certainly there will be more Byte Blocks 8 bits later.
//
// Note if a Byte Block is loaded, then the node passed in is replaced by
// the node loaded.
func (b *BPT) LoadNext(BIdx, bit byte, node *BptNode, key [32]byte) {
	if node.Left != nil && node.Left.T() == TNotLoaded ||
		node.Right != nil && node.Right.T() == TNotLoaded {
		b.Manager.LoadNode(node)
		_, ok1 := node.Left.(*NotLoaded)
		_, ok2 := node.Right.(*NotLoaded)
		if ok1 || ok2 {
			panic("We didn't load the node")
		}
	}
}

// Get
// Return the highest node that exists on the path to a particular node,
// and the entry along the path
func (b *BPT) Get(node *BptNode, key [32]byte) (highest *BptNode, entry *Entry, found bool) {

	BIdx := byte(node.Height >> 3) //          Calculate the byte index based on the height of this node in the BPT
	bitIdx := node.Height & 7      //          The bit index is given by the lower 3 bits of the height
	bit := byte(0x80) >> bitIdx    //          The mask starts at the high end bit in the byte, shifted right by the bitIdx

	entry = &node.Left      //                 Assume Left
	if bit&key[BIdx] == 0 { //                 Check for Right
		entry = &node.Right //                 Change to Right
	}

	b.LoadNext(BIdx, bit, node, key) //        Make sure following nodes are loaded

	switch { //                                Recurse down all the nodes
	case *entry == nil:
		return node, nil, false
	case (*entry).T() == TNode: //
		return b.Get((*entry).(*BptNode), key) //
	case (*entry).T() == TValue: //                           Look for value
		value := (*entry).(*Value)
		return node, entry, bytes.Equal(value.Key[:], key[:]) // Return true or false if value is found
	}
	panic("Should never reach this point")
}

// insertAtNode
// A recursive routine that pushes collisions towards the leaves of the
// binary patricia tree until the keys don't match any more.  Note that
// this tree cannot handle duplicate keys, but that is an assumption of
// patricia trees anyway
//
// Inputs:
// node -- the node in the BPT where the value (key, hash) is being inserted
// key  -- The key in the BPT which determines were in the BPT the hash goes
// hash -- The current value of the key, as tracked by the BPT
func (b *BPT) insertAtNode(node *BptNode, key, hash [32]byte) {

	BIdx := byte(node.Height >> 3) // Calculate the byte index based on the height of this node in the BPT
	bitIdx := node.Height & 7      // The bit index is given by the lower 3 bits of the height
	bit := byte(0x80) >> bitIdx    // The mask starts at the high end bit in the byte, shifted right by the bitIdx

	entry := &node.Left
	if bit&key[BIdx] == 0 { //                                       Check if heading left (the assumption)
		entry = &node.Right //                                       If wrong, heading
	}

	b.LoadNext(BIdx, bit, node, key)

	switch { //                                                      processing is done once here.
	case *entry == nil: //                                           Sort if the Left/Right is nil.
		v := b.NewValue(node, key, hash) //                          If it is, we can put the value here
		*entry = v                       //
		b.Dirty(node)                    //                          And changing the value of a node makes it dirty
		return                           //                          we are done.
	case (*entry).T() == TNode: //                                   If the entry isn't nil, check if it is a Node
		b.insertAtNode((*entry).(*BptNode), key, hash) //            Recurse up the tree
	default: //                                                      If not a node, not nil, it is a value.
		v := (*entry).(*Value)             //                        A collision. Get the value that got here first
		if bytes.Equal(key[:], v.Key[:]) { //                        If this value is the same as we are inserting
			if !bytes.Equal((*entry).(*Value).Hash[:], hash[:]) { // Make sure this is really a change, i.e.
				(*entry).(*Value).Hash = hash //                     the new hash is really different.  If it is
				b.Dirty(node)                 //                     mark the node as dirty
			}
			return //                                                Changed or not, we are done.
		} //                                                         The idea is to create a node, to replace the value
		nn := b.NewNode(key, node)                     //            that was here, and the old value and the new value
		*entry = nn                                    //            and insert them at one height higher.
		nn.NodeKey, _ = GetNodeKey(node.Height+1, key) //            Record the nn.BBKey
		b.insertAtNode(nn, key, hash)                  //            until they diverge.
		b.insertAtNode(nn, v.Key, v.Hash)              //            Because these are chainIDs, while they could be
	} //                                                             mined to attack our BPT, we don't much care; it will

}

// Insert
// Starts the search of the BPT for the location of the key in the BPT
func (b *BPT) Insert(key, hash [32]byte) { //          The location of a value is determined by the key, and the value
	b.insertAtNode(b.GetRoot(), key, hash) //          in that location is the hash.  We start at byte 0, lowest
} //                                                   significant bit. (which is masked with a 1)

// GetHash
// Makes the code just a bit more simple.  Checks for nils
func GetHash(e Entry) []byte {
	if e == nil { //              Sort for nil, return nil if e is nil.
		return nil
	}
	return e.GetHash() //         Otherwise, call the function to return the Hash for the entry.
}

// GetNodeHash
// Compute the hash of a node from its children
func GetNodeHash(n *BptNode) {
	L := GetHash(n.Left)  //                        Get the Left Branch
	R := GetHash(n.Right) //                        Get the Right Branch
	switch {              //                        Sort four conditions:
	case L != nil && R != nil: //                   If we have both L and R then combine
		n.Hash = sha256.Sum256(append(L, R...)) //  Take the hash of L+R
	case L != nil: //                               The next condition is where we only have L
		copy(n.Hash[:], L) //                       Just use L.  No hash required
	case R != nil: //                               Just have R.  Again, just use R.
		copy(n.Hash[:], R) //                       No Hash Required
	default: //                                     The fourth condition never happens, and bad if it does.
		panic("dead nodes should not exist") //     This is a node without a child somewhere up the tree.
	}
}

// Update the Patricia Tree hashes with the values from the
// updates since the last update
func (b *BPT) Update() error {
	for len(b.DirtyMap) > 0 { //                            While the DirtyMap has nodes to process
		dirtyList := b.GetDirtyList() //                    Get the Dirty List. Note sorted by height, High to low

		h := dirtyList[0].Height      //                    Get current height so we do one pass at one height at a time.
		for _, n := range dirtyList { //                    go through the list, and add parents to the dirty map
			if n.Height != h { //                           Note when the height is done,
				break //                                    bap out
			} //
			if h&b.Mask == 0 && b.Manager != nil { //       Sort and see if at the root node for a byte block
				err := b.Manager.FlushNode(n) //            If so, flush the byte block; it has already been updated
				if err != nil {
					return err
				}
			} //
			GetNodeHash(n)    //                            Set the node hash from its children
			b.Clean(n)        //                            Node has been updated, so it is clean
			b.Dirty(n.Parent) //                            The Parent is dirty cause it must consider this new state
		}
	}
	if b.Manager != nil { //                                Root doesn't get flushed (has no parent)
		b.Manager.Bpt.RootHash = b.Manager.Bpt.Root.Hash
		err := b.Manager.FlushNode(b.GetRoot()) //          So flush it special
		if err != nil {
			return err
		}
	} //
	b.RootHash = b.GetRoot().Hash //                        Set the root hash (so we don't have to load Root)
	return nil
}

func (b *BPT) EnsureRootHash() {
	n := b.GetRoot()      //                       Get the Root node
	L := GetHash(n.Left)  //                       Get the Left Branch
	R := GetHash(n.Right) //                       Get the Right Branch
	switch {              //                       Sort four conditions:
	case L != nil && R != nil: //                  If we have both L and R then combine
		n.Hash = sha256.Sum256(append(L, R...)) // Take the hash of L+R
	case L != nil: //                              The next condition is where we only have L
		copy(n.Hash[:], L) //                      Just use L.  No hash required
	case R != nil: //                              Just have R.  Again, just use R.
		copy(n.Hash[:], R) //                      No Hash Required
	}
}

// New BPT
// Allocate a new BPT and set up the structures required to get to work with
// Binary Patricia Trees.
func NewBPT(manager *Manager) *BPT {
	b := new(BPT)                                 // Get a Binary Patrica Tree
	b.Manager = manager                           // Point the BPT to the manager
	b.Power = 8                                   // using 4 bits to persist BPTs to disk
	b.Mask = b.Power - 1                          // Take the bits to the power of 2 -1
	b.Root = new(BptNode)                         // Allocate summary node (contributes nothing to BPT summary Hash
	b.Root.Height = 0                             // Before the next level
	b.Root.NodeKey, _ = GetNodeKey(0, [32]byte{}) // Set the zero height NodeKey
	b.DirtyMap = make(map[[32]byte]*BptNode)      // Allocate the Dirty Map, because batching updates is
	return b                                      // a pretty powerful way to process Patricia Trees
}

// MarshalByteBlock
// Given the node leading into a byte block, marshal all the nodes within the
// block.  A borderNode is a node that completes a byte boundary.  So consider
// a theoretical key 03e706b93d2e515c6eff056ee481eb92f9e790277db91eb748b3cc5b46dfe8ca
// The first byte is 03, second is a7, third is 06 etc.
//
// The node in block 03 that completes e7 is the board node.  The Left path
// would begin the path to the theoretical key (a bit zero).
func (b *BPT) MarshalByteBlock(borderNode *BptNode) (data []byte) {
	if borderNode.Height&b.Mask != 0 { //                Must be a boarder node
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
		entry.(*BptNode).Height&b.Mask == 0: //                   See if entry is going into
		data = append(data, TNode)              //             Mark as going into a node
		data = append(data, entry.Marshal()...) //             Put the fields into the slice
		data = append(data, TNotLoaded)         //             Left is going into next Byte Block
		data = append(data, TNotLoaded)         //             Right is going into next Byte Block
		return data                             //             Return the data
	case entry.T() == TNotLoaded: //                           Sort if node isn't loaded
		data = append(data, TNotLoaded)
	default: //                                                In this case, we have a node to marshal
		data = append(data, TNode)                          //    Mark as going into a node
		data = append(data, entry.Marshal()...)             //    Put the fields into the slice
		data = b.MarshalEntry(entry.(*BptNode).Left, data)  //    Marshal Left
		data = b.MarshalEntry(entry.(*BptNode).Right, data) //    Marshal Right
	}
	return data
}

// UnMarshalByteBlock
//
func (b *BPT) UnMarshalByteBlock(borderNode *BptNode, data []byte) []byte {
	if borderNode.Height&b.Mask != 0 {
		panic("cannot call UnMarshalByteBlock on non-boarder nodes")
	}
	borderNode.Left, data = b.UnMarshalEntry(borderNode, data)
	borderNode.Right, data = b.UnMarshalEntry(borderNode, data)
	return data
}

func (b *BPT) UnMarshalEntry(parent *BptNode, data []byte) (Entry, []byte) { //
	nodeType, data := data[0], data[1:] //      Pull the node type out of the slice
	switch nodeType {                   //      Based on node time, use proper unmarshal code
	case TNil: //                               If a nil, easy
		return nil, data //                     return the data pointer (sans the type)
	case TValue: //                             If a value
		v := new(Value)          //             unmarshal the value
		data = v.UnMarshal(data) //
		return v, data           //             Return the value object and updated data slice
	case TNotLoaded: //                         If not loaded
		if parent.Height&b.Mask != 0 {
			panic("writing a TNotLoaded node on a non-boundary node")
		}
		return new(NotLoaded), data //          Create the NotLoaded stub and updated pointer
	case TNode: //                              If a Node
		n := new(BptNode)                         // Allocate a new node
		n.Parent = parent                         // Set the Parent
		data = n.UnMarshal(data)                  // Get the Node
		n.Left, data = b.UnMarshalEntry(n, data)  // Populate Left
		n.Right, data = b.UnMarshalEntry(n, data) // Populate Right
		return n, data
	}
	panic("failure to decode the ByteBlock")
}

// GetNodeKey
// We need a key to address nodes in the protocol. These nodes need a unique key
// for debugging purposes.
// We return the key with height number of bits followed by a one end bit followed by all bits clear
// Heights greater than 255 (0-254) bits are not supported.
func GetNodeKey(height int, key [32]byte) (nodeKey [32]byte, ok bool) {
	if height > 254 { //                         Limit is 254 because one bit marks the end of the nodeKey
		return nodeKey, false //                Return a blank nodeKey and flag it didn't work
	} //
	byteCnt := height >> 3                   // The byte count is height/8 (shift left by 3)
	bitCnt := height & 7                     // Mask to the mod of 8 so mask with 7 or 0b111
	nk := append([]byte{}, key[:byteCnt]...) // Move the bytes into the node Key
	lastByte := key[byteCnt]                 // Get the byte following these bytes into lastByte
	lastByte >>= 7 - bitCnt                  //                Shift right all but one bit past the key
	lastByte |= 1                            //                Force that bit to 1
	lastByte <<= 7 - bitCnt                  //                Shift left back to the original starting point
	nk = append(nk, lastByte)                //                Add the last byte to the nk.  Note always add this byte
	copy(nodeKey[:], nk)                     //                Copy into the key array
	return nodeKey, true                     //                Return it as good, and that it works.
}

// GetHtKey
// Extract the height and Key fragment from a nodeKey.  The reverse operation of GetNodeKey
// Mostly useful for debugging and testing
func GetHtKey(nodeKey [32]byte) (height int, key [32]byte, ok bool) {
	copy(key[:], nodeKey[:])
	byteIdx := 0                             // Calculate the trailing bytes of zero
	for i := 31; i > 0 && key[i] == 0; i-- { // Look at byte 31 back to 0
		byteIdx++
	}
	byteIdx = 31 - byteIdx // Convert to bytes not zero

	lastByte := nodeKey[byteIdx]
	if lastByte == 0 {
		return height, key, false
	}
	bit := 1
	bitMask := byte(1)
	for lastByte&bitMask == 0 {
		bit++
		bitMask <<= 1
	}
	key[byteIdx] ^= bitMask
	return byteIdx*8 + 8 - bit, key, true
}

// GetLeftNodeKey
// Computes the nodeKey that a left branch from a BptNode must have
func GetChildrenNodeKeys(nodeKey [32]byte) (left, right [32]byte, ok bool) {
	copy(left[:], nodeKey[:])           // left is the same as nodeKey with the last bit set
	copy(right[:], nodeKey[:])          // right is the same as nodeKey with the last bit clear
	byteIdx := 0                        // Calculate the trailing bytes of zero
	for i := 31; nodeKey[i] == 0; i-- { // Look at byte 31 back to 0
		byteIdx++ //                       Count how many trailing bytes are zero
	}
	byteIdx = 31 - byteIdx //              Convert to leading bytes not zero

	lastByte := nodeKey[byteIdx] //        Get the first of the trailing zero bytes
	if lastByte == 0 {           //        If that is zero (i.e. no byte has a bit set)
		return left, right, false //         this is an invalid nodeKey
	}
	bit := 1                    //         Find the last bit set
	bitMask := byte(1)          //
	for lastByte&bitMask == 0 { //         For every zero low order bit
		bit++         //                   Increment count of bits clear
		bitMask <<= 1 //                   shift up the mask
	}
	bitsSet := 8 - bit            //       Bits set == 8 - bits clear
	height := byteIdx*8 + bitsSet //       Height is 8 bits for every non-zero byte + bitsSet

	right[byteIdx] ^= bitMask //           The nodeKey is actually the key fragment for the left
	//                                     While the right needs the end bit clear
	if left, ok = GetNodeKey(height+1, left); ok { // Calculate the Left; if good then
		right, ok = GetNodeKey(height+1, right) //    Calculate the Right
		return left, right, ok                  //    And return the fail flag from right
	} //
	return left, right, false //           Ah. The left failed; no need to look at the right
}
