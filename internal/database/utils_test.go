// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import "gitlab.com/accumulatenetwork/accumulate/internal/database/record"

func (b *Batch) ResolveAccountKey(key *record.Key) *record.Key {
	return b.resolveAccountKey(key)
}
