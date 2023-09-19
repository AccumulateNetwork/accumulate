// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

import (
	"encoding/json"
)

// equalNode is used to compare the values of the union
func equalNode(a, b node) bool {
	if a == b {
		return true
	}
	switch a := a.(type) {
	case *branch:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*branch)
		return ok && a.Equal(b)
	case *emptyNode:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*emptyNode)
		return ok && a.Equal(b)
	case *leaf:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*leaf)
		return ok && a.Equal(b)
	}
	return false
}

// copyNode copies a node.
func copyNode(v node) node {
	switch v := v.(type) {
	case *branch:
		return v.Copy()
	case *emptyNode:
		return v.Copy()
	case *leaf:
		return v.Copy()
	default:
		return v.CopyAsInterface().(node)
	}
}

// unmarshalNodeJson unmarshals a node.
func unmarshalNodeJSON(data []byte) (node, error) {
	var typ *struct{ Type nodeType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	if typ == nil {
		return nil, nil
	}

	acnt, err := newNode(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}
