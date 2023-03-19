// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package private

import (
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/badger"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
)

type KeyValueStore = storage.KeyValueStore
type KeyValueTxn = storage.KeyValueTxn

func NewMemoryStore(logger log.Logger) KeyValueStore {
	return memory.New(logger)
}

func NewBadgerStore(filepath string, logger log.Logger) (KeyValueStore, error) {
	return badger.New(filepath, logger)
}
