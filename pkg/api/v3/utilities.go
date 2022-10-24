package api

import "gitlab.com/accumulatenetwork/accumulate/internal/errors"

func RangeAs[U, V Record](r *RecordRange[V]) ([]U, error) {
	s := make([]U, len(r.Records))
	for i, v := range r.Records {
		u, ok := any(v).(U)
		if !ok {
			var z U
			return nil, errors.Format(errors.StatusConflict, "want %T, got %T", z, v)
		}
		s[i] = u
	}
	return s, nil
}

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
			return nil, errors.Wrap(errors.StatusUnknownError, err)
		}
		r.Records[i] = u
	}
	return r, nil
}

func ChainEntryAs[U, V Record](r *ChainEntryRecord[V]) (*ChainEntryRecord[U], error) {
	var zv V
	var zu U
	s := new(ChainEntryRecord[U])
	if u, ok := any(r.Value).(U); ok {
		s.Value = u
	} else if !EqualRecord(r.Value, zv) {
		return nil, errors.Format(errors.StatusConflict, "want %T, got %T", zu, r.Value)
	}

	s.Account = r.Account
	s.Name = r.Name
	s.Type = r.Type
	s.Index = r.Index
	s.Entry = r.Entry
	s.Receipt = r.Receipt
	return s, nil
}
