// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

import (
	"encoding/json"
	"fmt"
)

// NewPrivateKey creates a new PrivateKey for the specified PrivateKeyType.
func NewPrivateKey(typ PrivateKeyType) (PrivateKey, error) {
	switch typ {
	case PrivateKeyTypeCometNodeKeyFile:
		return new(CometNodeKeyFile), nil
	case PrivateKeyTypeCometPrivValFile:
		return new(CometPrivValFile), nil
	case PrivateKeyTypeSeed:
		return new(PrivateKeySeed), nil
	case PrivateKeyTypeRaw:
		return new(RawPrivateKey), nil
	case PrivateKeyTypeTransient:
		return new(TransientPrivateKey), nil
	}
	return nil, fmt.Errorf("unknown private key %v", typ)
}

// EqualPrivateKey is used to compare the values of the union
func EqualPrivateKey(a, b PrivateKey) bool {
	if a == b {
		return true
	}
	switch a := a.(type) {
	case *CometNodeKeyFile:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*CometNodeKeyFile)
		return ok && a.Equal(b)
	case *CometPrivValFile:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*CometPrivValFile)
		return ok && a.Equal(b)
	case *PrivateKeySeed:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*PrivateKeySeed)
		return ok && a.Equal(b)
	case *RawPrivateKey:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*RawPrivateKey)
		return ok && a.Equal(b)
	case *TransientPrivateKey:
		if a == nil {
			return b == nil
		}
		b, ok := b.(*TransientPrivateKey)
		return ok && a.Equal(b)
	}
	return false
}

// CopyPrivateKey copies a PrivateKey.
func CopyPrivateKey(v PrivateKey) PrivateKey {
	switch v := v.(type) {
	case *CometNodeKeyFile:
		return v.Copy()
	case *CometPrivValFile:
		return v.Copy()
	case *PrivateKeySeed:
		return v.Copy()
	case *RawPrivateKey:
		return v.Copy()
	case *TransientPrivateKey:
		return v.Copy()
	default:
		return v.CopyAsInterface().(PrivateKey)
	}
}

// UnmarshalPrivateKeyJson unmarshals a PrivateKey.
func UnmarshalPrivateKeyJSON(data []byte) (PrivateKey, error) {
	var typ *struct{ Type PrivateKeyType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	if typ == nil {
		return nil, nil
	}

	acnt, err := NewPrivateKey(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}