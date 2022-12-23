package bpt2

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

type nodeStatus int

const (
	// nodeClean indicates the node is unchanged since the last sync.
	nodeClean nodeStatus = iota

	// nodeDirty indicates the node has yet to be written to disk.
	nodeDirty

	// nodeOutOfSync indicates the node's hash is out of date.
	nodeOutOfSync
)

type Node struct {
	Height  uint64
	NodeKey [32]byte
	Hash    [32]byte

	key    record.Key
	status nodeStatus
	bpt    *BPT
	Parent *Node
	Left   Entry
	Right  Entry
}

func newNode(bpt *BPT, parent *Node) *Node {
	n := new(Node)
	n.bpt = bpt
	n.Parent = parent
	if parent != nil {
		n.bpt = parent.bpt
		n.Height = parent.Height + 1
	}
	return n
}

func newRootNode(bpt *BPT) *Node {
	n := newNode(bpt, nil)
	n.NodeKey, _ = nodeKeyAt(0, [32]byte{})
	n.Left = new(NotLoaded)
	n.Right = new(NotLoaded)
	n.register()
	return n
}

func newChildNode(parent *Node, key [32]byte) *Node {
	n := newNode(nil, parent)
	n.NodeKey, _ = nodeKeyAt(n.Height, key)
	n.Left = &NilEntry{parent: n}
	n.Right = &NilEntry{parent: n}
	n.register()
	return n
}

func readNode(parent *Node, rd io.Reader) (*Node, error) {
	n := newNode(nil, parent)
	err := n.UnmarshalBinaryFrom(rd)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	n.register()
	return n, nil
}

func (n *Node) withParent(parent *Node, recursive bool) Entry {
	m := newNode(nil, parent)
	m.status = nodeDirty
	m.Height = n.Height
	m.NodeKey = n.NodeKey
	m.Hash = n.Hash
	if recursive {
		m.Left = n.Left.withParent(m, true)
		m.Right = n.Right.withParent(m, true)
	} else {
		m.Left = new(NotLoaded)
		m.Right = new(NotLoaded)
	}
	m.register()
	return m
}

func (n *Node) register() {
	n.key = n.bpt.key.Append(n.NodeKey)
}

func (*Node) Type() EntryType { return EntryTypeNode }

// nodeKeyAt
// We need a key to address nodes in the protocol. These nodes need a unique key
// for debugging purposes.
// We return the key with height number of bits followed by a one end bit followed by all bits clear
// Heights greater than 255 (0-254) bits are not supported.
func nodeKeyAt(height uint64, key [32]byte) (nodeKey [32]byte, ok bool) {
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

// parseNodeKey
// Extract the height and Key fragment from a nodeKey.  The reverse operation of GetNodeKey
// Mostly useful for debugging and testing
func parseNodeKey(nodeKey [32]byte) (height uint64, key [32]byte, ok bool) {
	copy(key[:], nodeKey[:])
	byteIdx := uint64(0)                     // Calculate the trailing bytes of zero
	for i := 31; i > 0 && key[i] == 0; i-- { // Look at byte 31 back to 0
		byteIdx++
	}
	byteIdx = 31 - byteIdx // Convert to bytes not zero

	lastByte := nodeKey[byteIdx]
	if lastByte == 0 {
		return height, key, false
	}
	bit := uint64(1)
	bitMask := byte(1)
	for lastByte&bitMask == 0 {
		bit++
		bitMask <<= 1
	}
	key[byteIdx] ^= bitMask
	return byteIdx*8 + 8 - bit, key, true
}

func (n *Node) getAt(height uint64, key [32]byte) (*Node, error) {
again:
	if n.Height == height {
		return n, nil
	}

	err := n.load()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	BIdx := byte(n.Height >> 3) // Calculate the byte index based on the height of this node in the BPT
	bitIdx := n.Height & 7      // The bit index is given by the lower 3 bits of the height
	bit := byte(0x80) >> bitIdx // The mask starts at the high end bit in the byte, shifted right by the bitIdx

	side := "left"
	entry := n.Left
	if bit&key[BIdx] == 0 {
		side = "right"
		entry = n.Right
	}

	switch entry := entry.(type) {
	case *Node:
		n = entry
		goto again
	case *NilEntry:
		s := hex.EncodeToString(key[:])
		s = strings.TrimRight(s, "0")
		return nil, errors.NotFound.WithFormat("%s/%d not found", s, height)
	default:
		s := hex.EncodeToString(key[:])
		s = strings.TrimRight(s, "0")
		return nil, errors.BadRequest.WithFormat("cannot get %s/%d: entry %d/%s: expected %v, got %v", s, height, n.Height, side, EntryTypeNode, entry.Type())
	}
}

// load
// Load the next level of nodes if it is necessary.  We build Byte Blocks of
// nodes which store all the nodes and values within 8 bits of a key.  If the
// key is longer, certainly there will be more Byte Blocks 8 bits later.
//
// Note if a Byte Block is loaded, then the node passed in is replaced by
// the node loaded.
func (n *Node) load() error {
	if n.Left.Type() != EntryTypeNotLoaded &&
		n.Right.Type() != EntryTypeNotLoaded {
		return nil
	}

	err := n.bpt.store.GetValue(n.bpt.key.Append(n.NodeKey), nodeValue{n})
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		n.Left = &NilEntry{parent: n}
		n.Right = &NilEntry{parent: n}
	default:
		return errors.UnknownError.Wrap(err)
	}

	if n.Left.Type() == EntryTypeNotLoaded {
		return errors.InternalError.With("left is not loaded after loading")
	}
	if n.Right.Type() == EntryTypeNotLoaded {
		return errors.InternalError.With("right is not loaded after loading")
	}
	return nil
}

func (n *Node) Resolve(key record.Key) (record.Record, record.Key, error) {
	if len(key) > 0 {
		return nil, nil, errors.InternalError.With("bad key for bpt node")
	}
	return nodeValue{n}, nil, nil
}

// GetHash returns the node's hash, recalculating it if necessary.
func (n *Node) GetHash() []byte {
	if n.status != nodeOutOfSync {
		return n.Hash[:]
	}

	L := n.Left.GetHash()  //                      Get the Left Branch
	R := n.Right.GetHash() //                      Get the Right Branch
	switch {               //                      Sort four conditions:
	case L != nil && R != nil: //                  If we have both L and R then combine
		n.Hash = sha256.Sum256(append(L, R...)) // Take the hash of L+R
	case L != nil: //                              The next condition is where we only have L
		n.Hash = *(*[32]byte)(L) //                Just use L.  No hash required
	case R != nil: //                              Just have R.  Again, just use R.
		n.Hash = *(*[32]byte)(R) //                No Hash Required
	default: //                                    The fourth condition never happens, and bad if it does.
		panic("dead nodes should not exist") // This is a node without a child somewhere up the tree.
	}
	n.status = nodeDirty
	return n.Hash[:]
}

func (n *Node) IsDirty() bool { return n.status != nodeClean }

// Commit updates hashes and commits modifications to the underlying store. The
// node may be reused after committing. The exact behavior depends on the nature
// of the store.
//
// When writing to a key-value store, the guard around PutValue ensures only
// blocks are written, and the recursive Commit calls ensure all blocks are
// written.
//
// When writing to a parent batch, the entire tree is written out in a single
// PutValue call. The logic of [nodeValue.LoadValue] stops Commit from
// recursing.
func (n *Node) Commit() error {
	if n == nil || !n.IsDirty() {
		return nil
	}

	// Ensure hashes are up to date
	n.GetHash()

	// Write
	if n.Height&n.bpt.mask == 0 {
		err := n.bpt.store.PutValue(n.key, nodeValue{n})
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	// Commit children
	if m, ok := n.Left.(*Node); ok {
		err := m.Commit()
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	if m, ok := n.Right.(*Node); ok {
		err := m.Commit()
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	// Mark clean
	n.status = nodeClean
	return nil
}

type nodeValue struct{ *Node }

func (n nodeValue) GetValue() (encoding.BinaryValue, error) {
	return n, nil
}

func (n nodeValue) LoadBytes(data []byte) error {
	return n.UnmarshalBinary(data)
}

func (n nodeValue) LoadValue(value record.ValueReader, put bool) error {
	src, ok := value.(nodeValue)
	if !ok {
		return errors.InternalError.WithFormat("invalid value: expected %T, got %T", nodeValue{}, value)
	}

	if put {
		// Committing changes from a child batch into a parent batch.
		//
		//   1. Insert the modified child tree into the parent tree.
		_, err := n.insert(src.Node)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		//   2. Reload the child from the parent and prevent Commit from recursing.
		src.Hash = *(*[32]byte)(n.GetHash())
		src.Left = new(NotLoaded)
		src.Right = new(NotLoaded)
		return err
	}

	// Ensure the node is loaded
	err := src.load()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Force hashes to update
	src.GetHash()

	// Copy the source's left and right
	n.Left = src.Left.withParent(n.Node, false)
	n.Right = src.Right.withParent(n.Node, false)
	return nil
}

func (n nodeValue) MarshalBinary() ([]byte, error) {
	if n.Height&n.bpt.mask != 0 {
		return nil, errors.InternalError.With("invalid attempt to store non-border node")
	}
	return n.marshalChildren()
}

func (n nodeValue) UnmarshalBinary(data []byte) error {
	return n.UnmarshalBinaryFrom(bytes.NewBuffer(data))
}

func (n nodeValue) UnmarshalBinaryFrom(rd io.Reader) error {
	if n.Height&n.bpt.mask != 0 {
		return errors.InternalError.With("invalid attempt to load non-border node")
	}
	return n.unmarshalChildren(rd)
}
