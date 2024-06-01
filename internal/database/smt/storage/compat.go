// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package storage

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

const KeyLength = record.KeyHashLength

type Key = record.KeyHash

func MakeKey(v ...any) Key {
	return record.NewKey(v...).Hash()
}

// ErrNotFound is returned by KeyValueDB.Get if the key is not found.
var ErrNotFound = errors.NotFound
