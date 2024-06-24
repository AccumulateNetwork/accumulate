// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

//lint:file-ignore S1001,S1002,S1008,SA4013 generated code

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

type branch struct {
	bpt    *BPT
	parent *branch
	status branchStatus
	Height uint64   `json:"height,omitempty" form:"height" query:"height" validate:"required"`
	Key    [32]byte `json:"key,omitempty" form:"key" query:"key" validate:"required"`
	Hash   [32]byte `json:"hash,omitempty" form:"hash" query:"hash" validate:"required"`
	Left   node
	Right  node
}

type emptyNode struct {
	parent *branch
}

type leaf struct {
	Key    *record.Key `json:"key,omitempty" form:"key" query:"key" validate:"required"`
	Value  []byte      `json:"value,omitempty" form:"value" query:"value" validate:"required"`
	parent *branch
}

type parameters struct {
	RootHash  [32]byte `json:"rootHash,omitempty" form:"rootHash" query:"rootHash" validate:"required"`
	MaxHeight uint64   `json:"maxHeight,omitempty" form:"maxHeight" query:"maxHeight" validate:"required"`
	Power     uint64   `json:"power,omitempty" form:"power" query:"power" validate:"required"`
	Mask      uint64   `json:"mask,omitempty" form:"mask" query:"mask" validate:"required"`
}

func (v *branch) Copy() *branch {
	u := new(branch)

	u.Height = v.Height
	u.Key = v.Key
	u.Hash = v.Hash

	return u
}

func (v *branch) CopyAsInterface() interface{} { return v.Copy() }

func (v *emptyNode) Copy() *emptyNode {
	u := new(emptyNode)

	return u
}

func (v *emptyNode) CopyAsInterface() interface{} { return v.Copy() }

func (v *leaf) Copy() *leaf {
	u := new(leaf)

	if v.Key != nil {
		u.Key = (v.Key).Copy()
	}
	u.Value = encoding.BytesCopy(v.Value)

	return u
}

func (v *leaf) CopyAsInterface() interface{} { return v.Copy() }

func (v *parameters) Copy() *parameters {
	u := new(parameters)

	u.RootHash = v.RootHash
	u.MaxHeight = v.MaxHeight
	u.Power = v.Power
	u.Mask = v.Mask

	return u
}

func (v *parameters) CopyAsInterface() interface{} { return v.Copy() }

func (v *branch) Equal(u *branch) bool {
	if !(v.Height == u.Height) {
		return false
	}
	if !(v.Key == u.Key) {
		return false
	}
	if !(v.Hash == u.Hash) {
		return false
	}

	return true
}

func (v *emptyNode) Equal(u *emptyNode) bool {

	return true
}

func (v *leaf) Equal(u *leaf) bool {
	switch {
	case v.Key == u.Key:
		// equal
	case v.Key == nil || u.Key == nil:
		return false
	case !((v.Key).Equal(u.Key)):
		return false
	}
	if !(bytes.Equal(v.Value, u.Value)) {
		return false
	}

	return true
}

func (v *parameters) Equal(u *parameters) bool {
	if !(v.RootHash == u.RootHash) {
		return false
	}
	if !(v.MaxHeight == u.MaxHeight) {
		return false
	}
	if !(v.Power == u.Power) {
		return false
	}
	if !(v.Mask == u.Mask) {
		return false
	}

	return true
}
