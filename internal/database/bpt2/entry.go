package bpt2

import (
	"bytes"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// EntryType is the type of an [Entry].
type EntryType int

// Entry
// We only have two node types, a Node that builds the Patricia Tree, and
// a Value that holds the values at the leaves.
type Entry interface {
	encoding.BinaryValue
	Type() EntryType // Returns the type of entry
	GetHash() []byte // Returns the Hash for the entry
	// Equal(entry Entry) bool // Return Entry == entry

	insert(m Entry) (Entry, error)
	withParent(n *Node, recursive bool) Entry
}

func (v *Value) GetHash() []byte   { return v.Hash[:] }
func (*NilEntry) GetHash() []byte  { return nil }
func (*NotLoaded) GetHash() []byte { return nil }

const (
	TNil       = EntryTypeNil       // When persisting, this is the type for nils
	TNode      = EntryTypeNode      // Type for Nodes
	TValue     = EntryTypeValue     // Type for values
	TNotLoaded = EntryTypeNotLoaded // When transitioning into a new Byte Block, the NotLoaded indicates a need to load from disk
)

func unmarshalEntry(parent *Node, rd io.Reader) (Entry, error) {
	var buf [1]byte
	_, err := io.ReadFull(rd, buf[:])
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	switch typ := EntryType(buf[0]); typ {
	case EntryTypeNil:
		return &NilEntry{parent: parent}, nil

	case EntryTypeValue:
		v := new(Value)
		v.parent = parent
		err = v.UnmarshalBinaryFrom(rd)
		return v, errors.UnknownError.Wrap(err)

	case EntryTypeNotLoaded:
		if parent.Height&parent.bpt.mask != 0 {
			panic("writing a TNotLoaded node on a non-boundary node")
		}
		return new(NotLoaded), nil //          Create the NotLoaded stub and updated pointer

	case EntryTypeNode:
		n, err := readNode(parent, rd)
		return n, errors.UnknownError.Wrap(err)

	default:
		return nil, errors.EncodingError.WithFormat("invalid entry type %v", typ)
	}
}

func (*NotLoaded) insert(m Entry) (Entry, error) {
	panic("cannot insert - not loaded")
}

func (n *NotLoaded) withParent(*Node, bool) Entry {
	return n
}

func (*NilEntry) withParent(n *Node, recursive bool) Entry {
	return &NilEntry{parent: n}
}

func (v *Value) withParent(n *Node, recursive bool) Entry {
	return &Value{parent: n, Key: v.Key, Hash: v.Hash}
}

func (e *NilEntry) insert(m Entry) (Entry, error) {
	// Replace the nil entry
	return m.withParent(e.parent, true), nil
}

func (v *Value) insert(m Entry) (Entry, error) {
	// If the key and hash match, there's no change. If the key matches and the
	// hash is new, update the hash and return the value to indicate it has been
	// updated. If the key does not match, the value must be split.
	u, ok := m.(*Value)
	switch {
	case !ok || v.Key != u.Key:
		// Split
	case v.Hash != u.Hash:
		v.Hash = u.Hash
		return v, nil
	default:
		return nil, nil
	}

	n := newChildNode(v.parent, v.Key)
	_, err := n.insert(m)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	_, err = n.insert(v)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return n, nil
}

func (n *Node) insert(m Entry) (Entry, error) {
	err := n.load()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	switch m := m.(type) {
	case *Value:
		BIdx := byte(n.Height >> 3) // Calculate the byte index based on the height of this node in the BPT
		bitIdx := n.Height & 7      // The bit index is given by the lower 3 bits of the height
		bit := byte(0x80) >> bitIdx // The mask starts at the high end bit in the byte, shifted right by the bitIdx

		ptr := &n.Left
		if bit&m.Key[BIdx] == 0 { //                                       Check if heading left (the assumption)
			ptr = &n.Right //                                       If wrong, heading
		}

		new, err := (*ptr).insert(m)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		if new == nil {
			// Nothing changed
			return nil, nil
		}

		*ptr = new
		n.status = nodeOutOfSync
		return n, nil

	case *Node:
		if n.Height != m.Height {
			return nil, errors.InternalError.With("cannot merge nodes: height does not match")
		}
		if n.NodeKey != m.NodeKey {
			return nil, errors.InternalError.With("cannot merge nodes: key does not match")
		}

		// Ensure hashes are up to date
		n.GetHash()
		m.GetHash()

		if !bytes.Equal(n.Left.GetHash(), m.Left.GetHash()) {
			new, err := n.Left.insert(m.Left)
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}
			if new != nil {
				n.status = nodeOutOfSync
				n.Left = new
			}
		}

		if !bytes.Equal(n.Right.GetHash(), m.Right.GetHash()) {
			new, err := n.Right.insert(m.Right)
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}
			if new != nil {
				n.status = nodeOutOfSync
				n.Right = new
			}
		}

		if n.IsDirty() {
			return n, nil
		}
		return nil, nil

	default:
		panic(errors.InternalError.WithFormat("cannot merge a %v into a %v", m.Type(), EntryTypeNode))
	}
}
