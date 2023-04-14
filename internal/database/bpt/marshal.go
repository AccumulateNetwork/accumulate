// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// paramsStateSize is the marshaled size of [parameters].
const paramsStateSize = 1 + 2 + 2 + 32

// branchStateSize is the marshaled size of [branch].
const branchStateSize = 32 + 1 + 32

// leafStateSize is the marshaled size of [leaf].
const leafStateSize = 32 + 32

func (r *parameters) MarshalBinary() ([]byte, error) {
	// Marshal the fields
	var data []byte
	data = append(data, byte(r.MaxHeight))
	data = append(data, byte(r.Power>>8), byte(r.Power))
	data = append(data, byte(r.Mask>>8), byte(r.Mask))
	data = append(data, r.RootHash[:]...)
	return data, nil
}

func (r *parameters) UnmarshalBinary(data []byte) error {
	// Check the size
	if len(data) != paramsStateSize {
		return encoding.ErrNotEnoughData
	}

	// Unmarshal the fields
	r.MaxHeight = uint64(data[0])
	r.Power = uint64(data[1])<<8 + uint64(data[2])
	r.Mask = uint64(data[3])<<8 + uint64(data[4])
	r.RootHash = *(*[32]byte)(data[5:])
	return nil
}

func (r *parameters) UnmarshalBinaryFrom(rd io.Reader) error {
	// Read paramStateSize bytes
	var buf [paramsStateSize]byte
	_, err := io.ReadFull(rd, buf[:])
	if err != nil {
		return err
	}

	// Unmarshal
	return r.UnmarshalBinary(buf[:])
}

// tryWrite writes the bytes to the writer if err is nil. If the write returns
// an error, tryWrite assigns it to err.
func tryWrite(err *error, wr io.Writer, b []byte) {
	if *err == nil {
		_, *err = wr.Write(b)
	}
}

// tryWriteTo calls the writer if err is nil. If the writer returns an error,
// tryWriteTo assigns it to err.
func tryWriteTo(err *error, wr io.Writer, fn func(io.Writer) error) {
	if *err == nil {
		*err = fn(wr)
	}
}

func (*emptyNode) writeTo(io.Writer) error  { return nil }
func (*emptyNode) readFrom(io.Reader) error { return nil }

func (n *branch) writeTo(wr io.Writer) (err error) {
	// Write the fields
	tryWrite(&err, wr, n.Key[:])
	tryWrite(&err, wr, []byte{byte(n.Height)})
	tryWrite(&err, wr, n.Hash[:])
	return err
}

func (n *branch) readFrom(rd io.Reader) error {
	// Read branchStateSize bytes
	var buf [branchStateSize]byte
	_, err := io.ReadFull(rd, buf[:])
	if err != nil {
		return err
	}

	// Read the fields
	n.Key = *(*[32]byte)(buf[:])
	n.Height = uint64(buf[32])
	n.Hash = *(*[32]byte)(buf[33:])
	return nil
}

func (v *leaf) writeTo(wr io.Writer) (err error) {
	// Write the fields
	tryWrite(&err, wr, v.Key[:])
	tryWrite(&err, wr, v.Hash[:])
	return err
}

func (v *leaf) readFrom(rd io.Reader) error {
	// Read leafStateSize bytes
	var buf [leafStateSize]byte
	_, err := io.ReadFull(rd, buf[:])
	if err != nil {
		return err
	}

	// Read the fields
	v.Key = *(*[32]byte)(buf[:])
	v.Hash = *(*[32]byte)(buf[32:])
	return nil
}

// writeBlock writes the node as a block to the writer if err is nil. If writing
// produces an error, writeBlock assigns it to err.
func writeBlock(err *error, wr io.Writer, e node, mask uint64) {
	// Read the type and the entry's fields
	tryWrite(err, wr, []byte{byte(e.Type())})
	tryWriteTo(err, wr, e.writeTo)

	// We're done if the entry is not a branch
	br, ok := e.(*branch)
	if !ok {
		return
	}

	// If the branch is at the edge of a boundary, write the sentinel value for
	// it's left and right entries
	if br.Height&mask == 0 {
		tryWrite(err, wr, []byte{
			byte(nodeTypeBoundary),
			byte(nodeTypeBoundary),
		})
		return
	}

	// Record the left and right entries
	writeBlock(err, wr, br.Left, mask)
	writeBlock(err, wr, br.Right, mask)
}

// readNode recursively reads a node from the reader.
func readNode(rd io.Reader, parent *branch) (node, error) {
	// Read the node type
	var buf [1]byte
	_, err := io.ReadFull(rd, buf[:])
	if err != nil {
		return nil, err
	}

	// Stop if it's a boundary
	typ := nodeType(buf[0])
	if typ == nodeTypeBoundary {
		return nil, nil
	}

	// Create a node
	e, err := newNode(typ)
	if err != nil {
		return nil, err
	}

	// Set the parent
	switch e := e.(type) {
	case *emptyNode:
		e.parent = parent
	case *leaf:
		e.parent = parent
	case *branch:
		e.parent = parent
		e.bpt = parent.bpt
	default:
		return nil, errors.InternalError.WithFormat("unknown entry type %T", e)
	}

	// Read the node's fields
	err = e.readFrom(rd)
	if err != nil {
		return nil, err
	}

	// If the node is a branch, recurse left and right
	f, ok := e.(*branch)
	if !ok {
		return e, nil
	}

	f.Left, err = readNode(rd, f)
	if err != nil {
		return nil, err
	}

	f.Right, err = readNode(rd, f)
	if err != nil {
		return nil, err
	}

	return f, nil
}
