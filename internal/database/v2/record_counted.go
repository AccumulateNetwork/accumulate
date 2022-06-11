package database

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
)

type countableValue[T any] interface {
	Get() (T, error)
	Put(T) error
	record
}

type Counted[T any] struct {
	count  Wrapped[uint64]
	new    func(recordStore, recordKey, string) countableValue[T]
	values []countableValue[T]
}

func newCounted[T any](store recordStore, key recordKey, namefmt string, new func(recordStore, recordKey, string) countableValue[T]) *Counted[T] {
	c := &Counted[T]{}
	c.count = *newWrapped(store, key, namefmt, true, newWrapper(uintWrapper))
	c.new = new
	return c
}

func newCountableWrapped[T any](funcs *wrapperFuncs[T]) func(recordStore, recordKey, string) countableValue[T] {
	return func(store recordStore, key recordKey, namefmt string) countableValue[T] {
		return newWrapped(store, key, namefmt, false, newWrapper(funcs))
	}
}

//nolint:deadcode
func newCountableValue[T encoding.BinaryValue](new func() T) func(recordStore, recordKey, string) countableValue[T] {
	return func(store recordStore, key recordKey, namefmt string) countableValue[T] {
		return newValue(store, key, namefmt, false, new)
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

func (c *Counted[T]) isDirty() bool {
	if c == nil {
		return false
	}
	if c.count.isDirty() {
		return true
	}
	for _, v := range c.values {
		if v.isDirty() {
			return true
		}
	}
	return true
}

func (c *Counted[T]) commit() error {
	if c == nil {
		return nil
	}

	if err := c.count.commit(); err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	for _, v := range c.values {
		if err := v.commit(); err != nil {
			return errors.Wrap(errors.StatusUnknown, err)
		}
	}

	return nil
}

func (c *Counted[T]) resolve(key recordKey) (record, recordKey, error) {
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
