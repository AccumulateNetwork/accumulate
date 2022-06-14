package record

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
)

type countableValue[T any] interface {
	Get() (T, error)
	Put(T) error
	Record
}

type Counted[T any] struct {
	count  Wrapped[uint64]
	new    func(Store, Key, string) countableValue[T]
	values []countableValue[T]
}

func NewCounted[T any](store Store, key Key, namefmt string, new func(Store, Key, string) countableValue[T]) *Counted[T] {
	c := &Counted[T]{}
	c.count = *NewWrapped(store, key, namefmt, true, NewWrapper(UintWrapper))
	c.new = new
	return c
}

func NewCountableWrapped[T any](funcs *wrapperFuncs[T]) func(Store, Key, string) countableValue[T] {
	return func(store Store, key Key, namefmt string) countableValue[T] {
		return NewWrapped(store, key, namefmt, false, NewWrapper(funcs))
	}
}

//nolint:deadcode
func NewCountableValue[T encoding.BinaryValue](new func() T) func(Store, Key, string) countableValue[T] {
	return func(store Store, key Key, namefmt string) countableValue[T] {
		return NewValue(store, key, namefmt, false, new)
	}
}

func (c *Counted[T]) Count() (int, error) {
	v, err := c.count.Get()
	if err != nil {
		return 0, errors.Wrap(errors.StatusUnknown, err)
	}
	return int(v), nil
}

func (c *Counted[T]) value(i int) countableValue[T] {
	if len(c.values) < i+1 {
		c.values = append(c.values, make([]countableValue[T], i+1-len(c.values))...)
	}
	if c.values[i] != nil {
		return c.values[i]
	}

	key := c.count.key.Append(i)
	name := fmt.Sprintf("%s %d", c.count.name, i)
	v := c.new(c.count.store, key, name)
	c.values[i] = v
	return v
}

func (c *Counted[T]) Get(i int) (T, error) {
	v, err := c.value(i).Get()
	if err != nil {
		return v, errors.Wrap(errors.StatusUnknown, err)
	}

	return v, nil
}

func (c *Counted[T]) Put(v T) error {
	count, err := c.Count()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = c.count.Put(uint64(count + 1))
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = c.value(count).Put(v)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	return nil
}

func (c *Counted[T]) Last() (int, T, error) {
	count, err := c.Count()
	if err != nil {
		return 0, zero[T](), errors.Wrap(errors.StatusUnknown, err)
	}

	if count == 0 {
		return 0, zero[T](), errors.NotFound("empty")
	}

	v, err := c.Get(count - 1)
	if err != nil {
		return 0, zero[T](), errors.Wrap(errors.StatusInternalError, err)
	}

	return count - 1, v, nil
}

func (c *Counted[T]) IsDirty() bool {
	if c == nil {
		return false
	}
	if c.count.IsDirty() {
		return true
	}
	for _, v := range c.values {
		if v.IsDirty() {
			return true
		}
	}
	return true
}

func (c *Counted[T]) Commit() error {
	if c == nil {
		return nil
	}

	if err := c.count.Commit(); err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	for _, v := range c.values {
		if err := v.Commit(); err != nil {
			return errors.Wrap(errors.StatusUnknown, err)
		}
	}

	return nil
}

func (c *Counted[T]) Resolve(key Key) (Record, Key, error) {
	if len(key) == 0 {
		return &c.count, nil, nil
	}

	if len(key) > 1 {
		return nil, nil, errors.New(errors.StatusInternalError, "bad key for counted")
	}

	i, ok := key[0].(int)
	if !ok {
		return nil, nil, errors.New(errors.StatusInternalError, "bad key for value")
	}

	return c.value(i), nil, nil
}
