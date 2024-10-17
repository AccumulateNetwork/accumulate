// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func entry[V any](e *Entry[V]) ValueResult[V] {
	return value(e.Key, e.Value)
}

func value[V any](k *record.Key, v *Value[V]) ValueResult[V] {
	return ValueResult[V]{k, v}
}

func errResult[V any](err error) ErrorResult[V] {
	return ErrorResult[V]{err}
}

func notFound[V any](key *record.Key) ErrorResult[V] {
	return ErrorResult[V]{(*database.NotFoundError)(key)}
}

func rejectNotFound[V any](err error) ErrorResult[V] {
	if !errors.Is(err, errors.NotFound) {
		return errResult[V](err)
	}

	// Escalate NotFound to InternalError in cases where NotFound indicates a
	// corrupted index. For example, if the head references a block and that
	// block doesn't exist, that's a problem.
	return errResult[V](errors.InternalError.WithFormat("corrupted index: %w", err))
}
