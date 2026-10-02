// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"reflect"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
)

// databases is the one database each store of an instance is opened as. The
// section gate a joined node installs (executor spec, "Sync", §3a; #4485) is
// the database's, and every view of it -- Deep, Unisolated -- shares it; a
// second database.New over the same store would have a gate of its own that
// is never set, and every read through it of an account in a section not
// loaded would answer NotFound instead of being refused and redirected
// (#4497). So the executor, the API's querier, network and event services,
// the consensus, sequencer and proof services all take their database here.
type databases struct {
	mu  sync.Mutex
	dbs []openedDatabase
}

type openedDatabase struct {
	store keyvalue.Beginner
	db    *database.Database
}

// database is the database over store: the same one for every caller in this
// process, its subnodes included.
func (inst *Instance) database(store keyvalue.Beginner, logger logging.Logger) *database.Database {
	root := inst
	for root.parentInstance != nil {
		root = root.parentInstance
	}
	d := &root.databases
	d.mu.Lock()
	defer d.mu.Unlock()
	if reflect.TypeOf(store).Comparable() {
		for _, o := range d.dbs {
			if reflect.TypeOf(o.store) == reflect.TypeOf(store) && o.store == store {
				return o.db
			}
		}
	}
	db := database.New(store, logger)
	d.dbs = append(d.dbs, openedDatabase{store, db})
	return db
}
