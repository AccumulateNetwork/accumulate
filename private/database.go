// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package private

import (
	"io"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type Database = database.Database
type Batch = database.Batch
type Viewer = database.Viewer
type Updater = database.Updater
type Beginner = database.Beginner
type SnapshotHeader = snapshot.Header

const SnapshotVersion1 = snapshot.Version1

func NewDatabase(store KeyValueStore, logger log.Logger) *Database {
	return database.New(store, logger)
}

func CollectFullSnapshot(batch *database.Batch, file io.WriteSeeker, network *url.URL, logger log.Logger, preserve bool) error {
	return snapshot.FullCollect(batch, file, network, logger, preserve)
}

func OpenSnapshot(file SectionReader) (*snapshot.Header, *snapshot.Reader, error) {
	return snapshot.Open(file)
}
