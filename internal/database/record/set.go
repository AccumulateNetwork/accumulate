package record

import (
	"bytes"
	"io"
	"sort"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
)

type sliceableValue[T any] interface {
	encodableValue[[]T]
}

type Set[T any] struct {
	Value[[]T]
	compare func(u, v T) int
}

func NewSet[T any](logger log.Logger, store Store, key Key, namefmt string, value sliceableValue[T], cmp func(u, v T) int) *Set[T] {
	s := &Set[T]{}
	s.Value = *NewValue[[]T](logger, store, key, namefmt, true, value)
	s.compare = cmp
	return s
}

func (s *Set[T]) Put(u []T) error {
	// Sort it
	sort.Slice(u, func(i, j int) bool {
		return s.compare(u[i], u[j]) < 0
	})

	err := s.Value.Put(u)
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

	err = s.Value.Put(l)
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

	err = s.Value.Put(l)
	return errors.Wrap(errors.StatusUnknown, err)
}

func (s *Set[T]) Index(v T) (int, error) {
	l, err := s.Get()
	if err != nil {
		return 0, errors.Wrap(errors.StatusUnknown, err)
	}

	i, found := sortutil.Search(l, func(u T) int { return s.compare(u, v) })
	if !found {
		return 0, errors.NotFound("entry not found in %s", s.name)
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

func (s *Set[T]) IsDirty() bool {
	if s == nil {
		return false
	}
	return s.Value.IsDirty()
}

func (s *Set[T]) Commit() error {
	if s == nil {
		return nil
	}
	err := s.Value.Commit()
	return errors.Wrap(errors.StatusUnknown, err)
}

type slice[T encoding.BinaryValue] struct {
	value []T
	new   func() T
}

func NewSlice[T encoding.BinaryValue](new func() T) sliceableValue[T] {
	return &slice[T]{new: new}
}

func (v *slice[T]) getValue() []T  { return v.value }
func (v *slice[T]) setValue(u []T) { v.value = u }
func (v *slice[T]) setNew()        { v.value = nil }

func (v *slice[T]) copyValue() []T {
	if v.value == nil {
		return nil
	}
	u := make([]T, len(v.value))
	copy(u, v.value)
	return u
}

func (s *slice[T]) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	for _, v := range s.value {
		writer.WriteValue(1, v.MarshalBinary)
	}

	_, _, err := writer.Reset(nil)
	return buffer.Bytes(), errors.Wrap(errors.StatusUnknown, err)
}

func (s *slice[T]) UnmarshalBinary(data []byte) error {
	return s.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (s *slice[T]) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	for {
		v := s.new()
		if reader.ReadValue(1, v.UnmarshalBinary) {
			s.value = append(s.value, v)
		} else {
			break
		}
	}

	_, err := reader.Reset(nil)
	return errors.Wrap(errors.StatusUnknown, err)
}

func (s *slice[T]) CopyAsInterface() interface{} {
	t := new(slice[T])
	t.new = s.new
	t.value = s.copyValue()
	return t
}

type wrapperSlice[T any] struct {
	value []T
	*wrapperFuncs[T]
}

func NewWrapperSlice[T any](funcs *wrapperFuncs[T]) sliceableValue[T] {
	return &wrapperSlice[T]{wrapperFuncs: funcs}
}

func (v *wrapperSlice[T]) getValue() []T  { return v.value }
func (v *wrapperSlice[T]) setValue(u []T) { v.value = u }
func (v *wrapperSlice[T]) setNew()        { v.value = nil }

func (v *wrapperSlice[T]) copyValue() []T {
	if v.value == nil {
		return nil
	}
	u := make([]T, len(v.value))
	copy(u, v.value)
	return u
}

func (v *wrapperSlice[T]) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	for _, u := range v.value {
		writer.WriteValue(1, func() ([]byte, error) { return v.marshal(u) })
	}

	_, _, err := writer.Reset(nil)
	return buffer.Bytes(), errors.Wrap(errors.StatusUnknown, err)
}

func (v *wrapperSlice[T]) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *wrapperSlice[T]) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	for {
		ok := reader.ReadValue(1, func(data []byte) error {
			u, err := v.unmarshal(data)
			if err != nil {
				return err
			}
			v.value = append(v.value, u)
			return nil
		})
		if !ok {
			break
		}
	}

	_, err := reader.Reset(nil)
	return errors.Wrap(errors.StatusUnknown, err)
}

func (v *wrapperSlice[T]) CopyAsInterface() interface{} {
	w := *v
	w.value = v.copyValue()
	return &w
}
