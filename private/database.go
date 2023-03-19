// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package private

import (
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

type Database = database.Database
type Batch = database.Batch
type Viewer = database.Viewer
type Updater = database.Updater
type Beginner = database.Beginner

func NewDatabase(store KeyValueStore, logger log.Logger) *Database {
	return database.New(store, logger)
}
