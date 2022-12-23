// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package model

import (
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

func New(logger log.Logger, store record.Store, key record.Key, label string) *BPT {
	b := new(BPT)
	b.logger.Set(logger)
	b.store = store
	b.key = key
	b.label = label
	return b
}

func (b *BPT) Resolve(key record.Key) (record.Record, record.Key, error) {
	if len(key) == 0 {
		return nil, nil, errors.InternalError.With("bad key for bpt")
	}

	if key[0] == "Root" {
		return b.State(), key[1:], nil
	}

	nodeKey, ok := key[0].([32]byte)
	if !ok {
		return nil, nil, errors.InternalError.With("bad key for bpt")
	}
	return b.Block(nodeKey), key[1:], nil
}
