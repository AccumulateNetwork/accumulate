// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"strconv"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func LoadBlockLedger(ledger *database.Account, index uint64) (time.Time, []*protocol.BlockEntry, error) {
	_, l1, err := ledger.BlockLedger().Find(index).Exact().Get()
	switch {
	case err != nil && !errors.Is(err, errors.NotFound):
		return time.Time{}, nil, errors.UnknownError.Wrap(err)
	case err != nil, l1.Index == 0:
		// Not found or not populated
	default:
		return l1.Time, l1.Entries, nil
	}

	var l2 *protocol.BlockLedger
	err = ledger.Account(strconv.FormatUint(index, 10)).Main().GetAs(&l2)
	switch {
	case err != nil && !errors.Is(err, errors.NotFound):
		return time.Time{}, nil, errors.UnknownError.Wrap(err)
	case err != nil:
		// Not found
	default:
		return l2.Time, l2.Entries, nil
	}

	return time.Time{}, nil, errors.NotFound.WithFormat("cannot locate ledger for block %d", index)
}
