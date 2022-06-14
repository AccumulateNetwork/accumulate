package database

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
)

type Union[T encoding.BinaryValue] struct {
	value     T
	unmarshal valueUnmarshaller[T]
}

func newUnion[T encoding.BinaryValue](unmarshal valueUnmarshaller[T]) func() wrapperType[T] {
	return func() wrapperType[T] {
		u := new(Union[T])
		u.unmarshal = unmarshal
		return u
	}
}

func (v *Union[T]) getValue() T  { return v.value }
func (v *Union[T]) setValue(u T) { v.value = u }

func (v *Union[T]) MarshalBinary() ([]byte, error) {
	return v.value.MarshalBinary()
}

func (v *Union[T]) UnmarshalBinary(data []byte) error {
	u, err := v.unmarshal(data)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	v.value = u
	return nil
}

func (v *Union[T]) CopyAsInterface() interface{} {
	w := *v
	w.value = v.value.CopyAsInterface().(T)
	return &w
}

func (v *Union[T]) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = v.UnmarshalBinary(data)
	return errors.Wrap(errors.StatusUnknown, err)
}
