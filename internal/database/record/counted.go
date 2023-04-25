// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package record

import (
	"fmt"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type counted[T any] struct {
	count  *value[uint64]
	new    func() encodableValue[T]
	values []*value[T]
}

func newCounted[T any](logger log.Logger, store Store, key *Key, namefmt string, new func() encodableValue[T]) *counted[T] {
	c := &counted[T]{}
	c.count = newValue(logger, store, key, namefmt, true, Wrapped(UintWrapper))
	c.new = new
	return c
}

// Count loads the size of the list.
func (c *counted[T]) Count() (int, error) {
	v, err := c.count.Get()
	if err != nil {
		return 0, errors.UnknownError.Wrap(err)
	}
	return int(v), nil
}

func (c *counted[T]) value(i int) *value[T] {
	if len(c.values) < i+1 {
		c.values = append(c.values, make([]*value[T], i+1-len(c.values))...)
	}
	if c.values[i] != nil {
		return c.values[i]
	}

	key := c.count.key.Append(i)
	name := fmt.Sprintf("%s %d", c.count.name, i)
	v := newValue(c.count.logger.L, c.count.store, key, name, false, c.new())
	c.values[i] = v
	return v
}

// Get loads the I'th element of the list.
func (c *counted[T]) Get(i int) (T, error) {
	v, err := c.value(i).Get()
	if err != nil {
		return v, errors.UnknownError.Wrap(err)
	}

	return v, nil
}

// GetAll loads all the elements of the list.
func (c *counted[T]) GetAll() ([]T, error) {
	count, err := c.Count()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	values := make([]T, count)
	for i := range values {
		values[i], err = c.Get(i)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	return values, nil
}

// Put adds an element to the list.
func (c *counted[T]) Put(v T) error {
	count, err := c.Count()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = c.count.Put(uint64(count + 1))
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = c.value(count).Put(v)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return nil
}

// Last loads the value of the last element.
func (c *counted[T]) Last() (int, T, error) {
	count, err := c.Count()
	if err != nil {
		return 0, zero[T](), errors.UnknownError.Wrap(err)
	}

	if count == 0 {
		return 0, zero[T](), errors.NotFound.WithFormat("empty")
	}

	v, err := c.Get(count - 1)
	if err != nil {
		return 0, zero[T](), errors.InternalError.Wrap(err)
	}

	return count - 1, v, nil
}

// Overwrite overwrites the count and all entries. Overwrite will return an
// error if len(v) is less than the current count.
func (c *counted[T]) Overwrite(v []T) error {
	n, err := c.Count()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if len(v) < n {
		return errors.BadRequest.WithFormat("cannot overwrite with fewer values")
	}

	err = c.count.Put(uint64(len(v)))
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	for i, v := range v {
		err = c.value(i).Put(v)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	return nil
}

// IsDirty implements Record.IsDirty.
func (c *counted[T]) IsDirty() bool {
	if c == nil {
		return false
	}
	if c.count.IsDirty() {
		return true
	}
	for _, v := range c.values {
		if v == nil {
			continue
		}
		if v.IsDirty() {
			return true
		}
	}
	return true
}

// Commit implements Record.Commit.
func (c *counted[T]) Commit() error {
	if c == nil {
		return nil
	}

	if err := c.count.Commit(); err != nil {
		return errors.UnknownError.Wrap(err)
	}

	for _, v := range c.values {
		if v == nil {
			continue
		}
		if err := v.Commit(); err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	return nil
}

// Resolve implements Record.Resolve.
func (c *counted[T]) Resolve(key *Key) (Record, *Key, error) {
	if key.Len() == 0 {
		return c.count, nil, nil
	}

	if key.Len() > 1 {
		return nil, nil, errors.InternalError.With("bad key for counted")
	}

	i, ok := key.Get(0).(int)
	if !ok {
		return nil, nil, errors.InternalError.With("bad key for value")
	}

	return c.value(i), nil, nil
}

func (c *counted[T]) WalkChanges(fn WalkFunc) error {
	err := c.count.WalkChanges(fn)
	if err != nil {
		return err
	}
	for _, v := range c.values {
		err = v.WalkChanges(fn)
		if err != nil {
			return err
		}
	}
	return nil
}
