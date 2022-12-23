package bpt2

import (
	"bytes"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

const rootStateSize = 1 + 2 + 2 + 32
const nodeStateSize = 32 + 1 + 32
const valueStateSize = 32 + 32

func (r *RootState) MarshalBinary() ([]byte, error) {
	var data []byte
	data = append(data, byte(r.MaxHeight))
	data = append(data, byte(r.Power>>8), byte(r.Power))
	data = append(data, byte(r.Mask>>8), byte(r.Mask))
	data = append(data, r.RootHash[:]...)
	return data, nil
}

func (r *RootState) UnmarshalBinary(data []byte) error {
	if len(data) != rootStateSize {
		return encoding.ErrNotEnoughData
	}
	r.MaxHeight = uint64(data[0])
	r.Power = uint64(data[1])<<8 + uint64(data[2])
	r.Mask = uint64(data[3])<<8 + uint64(data[4])
	r.RootHash = *(*[32]byte)(data[5:])
	return nil
}

func (r *RootState) UnmarshalBinaryFrom(rd io.Reader) error {
	var buf [rootStateSize]byte
	_, err := io.ReadFull(rd, buf[:])
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(buf[:])
}

func (v *Value) MarshalBinary() ([]byte, error) {
	var data []byte
	data = append(data, v.Key[:]...)
	data = append(data, v.Hash[:]...)
	return data, nil
}

func (v *Value) UnmarshalBinary(data []byte) error {
	if len(data) != valueStateSize {
		return encoding.ErrNotEnoughData
	}
	v.Key = *(*[32]byte)(data)
	v.Hash = *(*[32]byte)(data[32:])
	return nil
}

func (v *Value) UnmarshalBinaryFrom(rd io.Reader) error {
	var buf [valueStateSize]byte
	_, err := io.ReadFull(rd, buf[:])
	if err != nil {
		return err
	}
	return v.UnmarshalBinary(buf[:])
}

func (*NilEntry) MarshalBinary() ([]byte, error)         { return nil, nil }
func (*NilEntry) UnmarshalBinary(data []byte) error      { return nil }
func (*NilEntry) UnmarshalBinaryFrom(rd io.Reader) error { return nil }

func (*NotLoaded) MarshalBinary() ([]byte, error)         { return nil, nil }
func (*NotLoaded) UnmarshalBinary(data []byte) error      { return nil }
func (*NotLoaded) UnmarshalBinaryFrom(rd io.Reader) error { return nil }

func (n *Node) Copy() *Node {
	n.GetHash()

	m := new(Node)
	m.Height = n.Height
	m.NodeKey = n.NodeKey
	m.Hash = n.Hash
	m.Left = n.Left.CopyAsInterface().(Entry)
	m.Right = n.Right.CopyAsInterface().(Entry)

	if o, ok := m.Left.(*Node); ok {
		o.Parent = m
	}
	if o, ok := m.Right.(*Node); ok {
		o.Parent = m
	}

	return m
}

func (n *Node) CopyAsInterface() interface{} { return n.Copy() }

// func (n *Node) Equal(u *Node) bool {
// 	n.GetHash()
// 	u.GetHash()
// 	return true &&
// 		n.Height == u.Height &&
// 		n.NodeKey == u.NodeKey&&
// 		n.Hash == u.Hash &&
// 		n.
// }

func (n *Node) MarshalBinary() ([]byte, error) {
	var data []byte
	data = append(data, n.NodeKey[:]...)
	data = append(data, byte(n.Height))
	data = append(data, n.Hash[:]...)

	if n.Height&n.bpt.mask == 0 {
		data = append(data, byte(EntryTypeNotLoaded))
		data = append(data, byte(EntryTypeNotLoaded))
		return data, nil
	}

	b, err := n.marshalChildren()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	data = append(data, b...)
	return data, nil
}

func (n *Node) marshalChildren() ([]byte, error) {
	var data []byte
	data = append(data, byte(n.Left.Type()))
	b, err := n.Left.MarshalBinary()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	data = append(data, b...)

	data = append(data, byte(n.Right.Type()))
	b, err = n.Right.MarshalBinary()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	data = append(data, b...)

	return data, nil
}

func (n *Node) UnmarshalBinary(data []byte) error {
	return n.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (n *Node) UnmarshalBinaryFrom(rd io.Reader) error {
	var buf [nodeStateSize]byte
	_, err := io.ReadFull(rd, buf[:])
	if err != nil {
		return err
	}
	n.NodeKey = *(*[32]byte)(buf[:])
	n.Height = uint64(buf[32])
	n.Hash = *(*[32]byte)(buf[33:])

	return n.unmarshalChildren(rd)
}

func (n *Node) unmarshalChildren(rd io.Reader) error {
	var err error
	n.Left, err = unmarshalEntry(n, rd)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	n.Right, err = unmarshalEntry(n, rd)
	return errors.UnknownError.Wrap(err)
}
