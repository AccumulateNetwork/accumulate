// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package remote

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum  --package remote enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package remote types.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package remote --out unions_gen.go --language go-union types.yml

type callType int
type responseType int

type call interface {
	encoding.UnionValue
	Type() callType
}

type response interface {
	encoding.UnionValue
	Type() responseType
}

func wrap(key *record.Key) keyOrHash {
	if key.Len() == 0 {
		return keyOrHash{}
	}

	// If the key starts with a hash, convert the whole thing to a hash
	if _, ok := key.Get(0).(record.KeyHash); ok {
		return keyOrHash{Hash: key.Hash()}
	}

	return keyOrHash{Key: key}
}

func (k keyOrHash) unwrap() *record.Key {
	if k.Key != nil {
		return k.Key
	}
	return record.KeyFromHash(k.Hash)
}
