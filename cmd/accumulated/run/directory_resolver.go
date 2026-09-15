// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"strings"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// newDirectoryResolver builds the resolver the proof service uses to reach the
// directory's database (#4274). Every Accumulate node runs the directory
// alongside its own BVN, so both halves of an account proof are local -- but
// the two partitions start independently and either order is legal, so the
// lookup is deferred to the first call rather than done at registration.
//
// Only success is cached. Caching the failure would let one call made before
// the directory registered its storage poison every later call, permanently,
// which is the ordering problem the deferral exists to avoid.
func newDirectoryResolver(partition string, own database.Viewer, open func() (database.Viewer, error)) func() (database.Viewer, error) {
	var mu sync.Mutex
	var db database.Viewer
	return func() (database.Viewer, error) {
		mu.Lock()
		defer mu.Unlock()
		if db != nil {
			return db, nil
		}
		if strings.EqualFold(partition, protocol.Directory) {
			db = own
			return db, nil
		}
		v, err := open()
		if err != nil {
			return nil, err
		}
		db = v
		return db, nil
	}
}
