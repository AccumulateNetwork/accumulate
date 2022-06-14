package database

import (
	"bytes"
	"io"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
)

type sliceableValue[T any] interface {
	encoding.BinaryValue
	getValues() *[]T
}

type Set[T any] struct {
	Value[sliceableValue[T]]
	compare func(u, v T) int
}

func newSet[T any](store recordStore, key recordKey, namefmt string, new func() sliceableValue[T], cmp func(u, v T) int) *Set[T] {
	s := &Set[T]{}
	s.Value = *newValue(store, key, namefmt, true, new)
	s.compare = cmp
	return s
}

func (s *Set[T]) Get() ([]T, error) {
	l, err := s.Value.Get()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	return *l.getValues(), nil
}

func (s *Set[T]) GetAs(target interface{}) error {
	l, err := s.Value.Get()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = encoding.SetPtr(*l.getValues(), target)
	return errors.Wrap(errors.StatusUnknown, err)
}

func (s *Set[T]) Put(u []T) error {
	l, err := s.Value.Get()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	// Sort it
	sort.Slice(u, func(i, j int) bool {
		return s.compare(u[i], u[j]) < 0
	})

	*l.getValues() = u
	err = s.Value.Put(l)
	return errors.Wrap(errors.StatusUnknown, err)
}

func (s *Set[T]) Add(v ...T) error {
	l, err := s.Get()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	for _, v := range v {
		ptr, _ := sortutil.BinaryInsert(&l, func(u T) int { return s.compare(u, v) })
		*ptr = v
	}

	err = s.Put(l)
	return errors.Wrap(errors.StatusUnknown, err)
}

func (s *Set[T]) Remove(v T) error {
	l, err := s.Get()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	i, found := sortutil.Search(l, func(u T) int { return s.compare(u, v) })
	if !found {
		return nil
	}
	l = append(l[:i], l[i+1:]...)

	err = s.Put(l)
	return errors.Wrap(errors.StatusUnknown, err)
}

func (s *Set[T]) Index(v T) (int, error) {
	l, err := s.Get()
	if err != nil {
		return 0, errors.Wrap(errors.StatusUnknown, err)
	}

	i, found := sortutil.Search(l, func(u T) int { return s.compare(u, v) })
	if !found {
		return 0, errors.NotFound("%s not found", s.name)
	}

	return i, nil
}

func (s *Set[T]) Find(v T) (T, error) {
	l, err := s.Get()
	if err != nil {
		return zero[T](), errors.Wrap(errors.StatusUnknown, err)
	}

	i, found := sortutil.Search(l, func(u T) int { return s.compare(u, v) })
	if !found {
		return zero[T](), errors.NotFound("%s entry not found", s.name)
	}

	return l[i], nil
}

func (s *Set[T]) isDirty() bool {
	if s == nil {
		return false
	}
	return s.Value.isDirty()
}

func (s *Set[T]) commit() error {
	if s == nil {
		return nil
	}
	err := s.Value.commit()
	return errors.Wrap(errors.StatusUnknown, err)
}

type Slice[T encoding.BinaryValue] struct {
	values []T
	new    func() T
}

func newSlice[T encoding.BinaryValue](new func() T) func() sliceableValue[T] {
	return func() sliceableValue[T] {
		s := &Slice[T]{}
		s.new = new
		return s
	}
}

func (s *Slice[T]) getValues() *[]T {
	return &s.values
}

func (s *Slice[T]) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	for _, v := range s.values {
		writer.WriteValue(1, v.MarshalBinary)
	}

	_, _, err := writer.Reset(nil)
	return buffer.Bytes(), errors.Wrap(errors.StatusUnknown, err)
}

func (s *Slice[T]) UnmarshalBinary(data []byte) error {
	return s.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (s *Slice[T]) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	for {
		v := s.new()
		if reader.ReadValue(1, v.UnmarshalBinary) {
			s.values = append(s.values, v)
		} else {
			break
		}
	}

	_, err := reader.Reset(nil)
	return errors.Wrap(errors.StatusUnknown, err)
}

func (s *Slice[T]) CopyAsInterface() interface{} {
	t := new(Slice[T])
	t.new = s.new
	t.values = make([]T, len(s.values))
	for i, v := range s.values {
		t.values[i] = v.CopyAsInterface().(T)
	}
	return t
}

type WrapperSlice[T any] struct {
	values []T
	*wrapperFuncs[T]
}

func newWrapperSlice[T any](funcs *wrapperFuncs[T]) func() sliceableValue[T] {
	return func() sliceableValue[T] {
		w := &WrapperSlice[T]{}
		w.wrapperFuncs = funcs
		return w
	}
}

func (s *WrapperSlice[T]) getValues() *[]T {
	return &s.values
}

func (v *WrapperSlice[T]) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	for _, u := range v.values {
		writer.WriteValue(1, func() ([]byte, error) { return v.marshal(u) })
	}

	_, _, err := writer.Reset(nil)
	return buffer.Bytes(), errors.Wrap(errors.StatusUnknown, err)
}

func (v *WrapperSlice[T]) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *WrapperSlice[T]) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	for {
		ok := reader.ReadValue(1, func(data []byte) error {
			u, err := v.unmarshal(data)
			if err != nil {
				return err
			}
			v.values = append(v.values, u)
			return nil
		})
		if !ok {
			break
		}
	}

	_, err := reader.Reset(nil)
	return errors.Wrap(errors.StatusUnknown, err)
}

func (v *WrapperSlice[T]) CopyAsInterface() interface{} {
	w := *v
	w.values = make([]T, len(v.values))
	for i, u := range v.values {
		w.values[i] = v.copy(u)
	}
	return &w
}
