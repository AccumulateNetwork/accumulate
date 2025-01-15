// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package values

//lint:file-ignore U1000 false positive

import (
	"bytes"
	"io"
	"sort"

	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

type set[T any] struct {
	value[[]T]
	compare func(u, v T) int
}

// NewSet returns a new set using the given encoder and comparison.
func newSet[T any](store database.Store, key *database.Key, encoder encodableValue[T], cmp func(u, v T) int) *set[T] {
	s := &set[T]{}
	s.value = *newValue[[]T](store, key, true, &sliceValue[T]{encoder: encoder})
	s.compare = cmp
	return s
}

// Put stores the value, sorted.
func (s *set[T]) Put(u []T) error {
	// Sort it
	sort.Slice(u, func(i, j int) bool {
		return s.compare(u[i], u[j]) < 0
	})

	err := s.value.Put(u)
	return errors.UnknownError.Wrap(err)
}

// Add inserts values into the set, sorted.
func (s *set[T]) Add(v ...T) error {
	l, err := s.Get()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	for _, v := range v {
		ptr, _ := sortutil.BinaryInsert(&l, func(u T) int { return s.compare(u, v) })
		*ptr = v
	}

	err = s.value.Put(l)
	return errors.UnknownError.Wrap(err)
}

// Remove removes a value from the set.
func (s *set[T]) Remove(v T) error {
	l, err := s.Get()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	i, found := sortutil.Search(l, func(u T) int { return s.compare(u, v) })
	if !found {
		return nil
	}
	l = append(l[:i], l[i+1:]...)

	err = s.value.Put(l)
	return errors.UnknownError.Wrap(err)
}

// Index returns the index of the value.
func (s *set[T]) Index(v T) (int, error) {
	l, err := s.Get()
	if err != nil {
		return 0, errors.UnknownError.Wrap(err)
	}

	i, found := sortutil.Search(l, func(u T) int { return s.compare(u, v) })
	if !found {
		return 0, (*database.NotFoundError)(s.key.Append("(Index)"))
	}

	return i, nil
}

// Find returns the matching value.
func (s *set[T]) Find(v T) (T, error) {
	l, err := s.Get()
	if err != nil {
		return zero[T](), errors.UnknownError.Wrap(err)
	}

	i, found := sortutil.Search(l, func(u T) int { return s.compare(u, v) })
	if !found {
		return zero[T](), (*database.NotFoundError)(s.key.Append("(Find)"))
	}

	return l[i], nil
}

// IsDirty implements Record.IsDirty.
func (s *set[T]) IsDirty() bool {
	if s == nil {
		return false
	}
	return s.value.IsDirty()
}

// Commit implements Record.Commit.
func (s *set[T]) Commit() error {
	if s == nil {
		return nil
	}
	err := s.value.Commit()
	return errors.UnknownError.Wrap(err)
}

func (v *set[T]) Walk(opts database.WalkOptions, fn database.WalkFunc) error {
	if opts.Modified && !v.IsDirty() {
		return nil
	}

	// If the set is empty, skip it
	u, err := v.Get()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if len(u) == 0 && !opts.Modified {
		return nil
	}

	// Walk the record
	_, err = fn(v)
	return err
}

// sliceValue uses an encoder to manage a slice.
type sliceValue[T any] struct {
	value   []T
	encoder encodableValue[T]
}

var _ encodableValue[[]string] = (*sliceValue[string])(nil)

func (v *sliceValue[T]) getValue() []T  { return v.value }
func (v *sliceValue[T]) setValue(u []T) { v.value = u }
func (v *sliceValue[T]) setNew()        { v.value = nil }

func (v *sliceValue[T]) copyValue() []T {
	if v.value == nil {
		return nil
	}
	u := make([]T, len(v.value))
	for i, w := range v.value {
		v.encoder.setValue(w)
		u[i] = v.encoder.copyValue()
	}
	return u
}

func (v *sliceValue[T]) CopyAsInterface() interface{} {
	u := new(sliceValue[T])
	u.value = v.copyValue()
	return u
}

func (v *sliceValue[T]) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)
	marshalSlice(writer, v.encoder, v.value)
	_, _, err := writer.Reset(nil)
	return buffer.Bytes(), errors.UnknownError.Wrap(err)
}

func (v *sliceValue[T]) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *sliceValue[T]) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)
	v.value = unmarshalSlice(reader, v.encoder)
	_, err := reader.Reset(nil)
	return errors.UnknownError.Wrap(err)
}

// marshalSlice uses an encodable value to marshal a slice. If this
// implementation were embedded in sliceValue.MarshalBinary, it would be easy to
// accidentally use the sliceValue instead of the encoder.
func marshalSlice[T any](wr *encoding.Writer, e encodableValue[T], v []T) {
	for _, u := range v {
		e.setValue(u)
		wr.WriteValue(1, e.MarshalBinary)
	}
}

// unmarshalSlice uses an encodable value to unmarshal a slice. See marshalSlice
// for why this is a separate function.
func unmarshalSlice[T any](rd *encoding.Reader, e encodableValue[T]) []T {
	var v []T
	for {
		e.setNew()
		if rd.ReadValue(1, e.UnmarshalBinaryFrom) {
			v = append(v, e.getValue())
		} else {
			break
		}
	}
	return v
}
