// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

func ChainEntryRecordAsMessage[T messaging.Message](r *ChainEntryRecord[Record]) (*ChainEntryRecord[*MessageRecord[T]], error) {
	m, err := ChainEntryRecordAs[*MessageRecord[messaging.Message]](r)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	r.Value, err = MessageRecordAs[T](m.Value)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	r2, err := ChainEntryRecordAs[*MessageRecord[T]](r)
	return r2, errors.UnknownError.Wrap(err)
}
