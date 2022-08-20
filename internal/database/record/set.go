package record

//lint:file-ignore U1000 false positive

import (
	"bytes"
	"io"
	"sort"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// Set records an ordered list of values as a single record.
type Set[T any] struct {
	Value[[]T]
	compare func(u, v T) int
}

// NewSet returns a new Set using the given encoder and comparison.
func NewSet[T any](logger log.Logger, store Store, key Key, namefmt string, encoder encodableValue[T], cmp func(u, v T) int) *Set[T] {
	s := &Set[T]{}
	s.Value = *NewValue[[]T](logger, store, key, namefmt, true, &sliceValue[T]{encoder: encoder})
	s.compare = cmp
	return s
}

// Put stores the value, sorted.
func (s *Set[T]) Put(u []T) error {
	// Sort it
	sort.Slice(u, func(i, j int) bool {
		return s.compare(u[i], u[j]) < 0
	})

	err := s.Value.Put(u)
	return errors.Unknown.Wrap(err)
}

// Add inserts values into the set, sorted.
func (s *Set[T]) Add(v ...T) error {
	l, err := s.Get()
	if err != nil {
		return errors.Unknown.Wrap(err)
	}

	for _, v := range v {
		ptr, _ := sortutil.BinaryInsert(&l, func(u T) int { return s.compare(u, v) })
		*ptr = v
	}

	err = s.Value.Put(l)
	return errors.Unknown.Wrap(err)
}

// Remove removes a value from the set.
func (s *Set[T]) Remove(v T) error {
	l, err := s.Get()
	if err != nil {
		return errors.Unknown.Wrap(err)
	}

	i, found := sortutil.Search(l, func(u T) int { return s.compare(u, v) })
	if !found {
		return nil
	}
	l = append(l[:i], l[i+1:]...)

	err = s.Value.Put(l)
	return errors.Unknown.Wrap(err)
}

// Index returns the index of the value.
func (s *Set[T]) Index(v T) (int, error) {
	l, err := s.Get()
	if err != nil {
		return 0, errors.Unknown.Wrap(err)
	}

	i, found := sortutil.Search(l, func(u T) int { return s.compare(u, v) })
	if !found {
		return 0, errors.NotFound.Format("entry not found in %s", s.name)
	}

	return i, nil
}

// Find returns the matching value.
func (s *Set[T]) Find(v T) (T, error) {
	l, err := s.Get()
	if err != nil {
		return zero[T](), errors.Unknown.Wrap(err)
	}

	i, found := sortutil.Search(l, func(u T) int { return s.compare(u, v) })
	if !found {
		return zero[T](), errors.NotFound.Format("%s entry not found", s.name)
	}

	return l[i], nil
}

// IsDirty implements Record.IsDirty.
func (s *Set[T]) IsDirty() bool {
	if s == nil {
		return false
	}
	return s.Value.IsDirty()
}

// Commit implements Record.Commit.
func (s *Set[T]) Commit() error {
	if s == nil {
		return nil
	}
	err := s.Value.Commit()
	return errors.Unknown.Wrap(err)
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
	return buffer.Bytes(), errors.Unknown.Wrap(err)
}

func (v *sliceValue[T]) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *sliceValue[T]) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)
	v.value = unmarshalSlice(reader, v.encoder)
	_, err := reader.Reset(nil)
	return errors.Unknown.Wrap(err)
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
		if rd.ReadValue(1, e.UnmarshalBinary) {
			v = append(v, e.getValue())
		} else {
			break
		}
	}
	return v
}
