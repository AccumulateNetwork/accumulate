// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

func MapRange[U, V Record](r *RecordRange[V], fn func(V) (U, error)) (*RecordRange[U], error) {
	s := new(RecordRange[U])
	s.Start = r.Start
	s.Total = r.Total
	s.Records = make([]U, len(r.Records))
	var err error
	for i, v := range r.Records {
		s.Records[i], err = fn(v)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

// MakeRange creates a record range with at most max elements of v (unless max
// is zero), transformed by fn. MakeRange only returns an error if fn returns an
// error.
//
// MakeRange will not return an error as long as fn does not.
func MakeRange[V any, U Record](values []V, start, count uint64, fn func(V) (U, error)) (*RecordRange[U], error) {
	r := new(RecordRange[U])
	r.Start = start
	r.Total = uint64(len(values))

	if start >= uint64(len(values)) {
		return r, nil
	}
	values = values[start:]

	if count > 0 && uint64(len(values)) > count {
		values = values[:count]
	}

	r.Records = make([]U, len(values))
	for i, v := range values {
		u, err := fn(v)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		r.Records[i] = u
	}
	return r, nil
}

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
