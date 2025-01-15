// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package values

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type keyType[T any] interface {
	ForMap() T
}

// GetOrCreate retrieves an existing record or creates, records, and assigns a
// new record to the field.
func GetOrCreate[Container any, Record any](c Container, ptr *Record, create func(Container) Record) Record {
	var z Record
	if any(*ptr) != any(z) {
		return *ptr
	}

	*ptr = create(c)
	return *ptr
}

// GetOrCreateMap retrieves an existing record or creates, records, and assigns
// a new record to the map.
func GetOrCreateMap[Container any, Record any, Key keyType[MapKey], MapKey comparable](c Container, ptr *map[MapKey]Record, key Key, create func(Container, Key) Record) Record {
	if *ptr == nil {
		*ptr = map[MapKey]Record{}
	}

	mk := key.ForMap()
	if v, ok := (*ptr)[mk]; ok {
		return v
	}

	v := create(c, key)
	(*ptr)[mk] = v
	return v
}

// Commit commits the record, if it is non-nil.
func Commit[T database.Record](lastErr *error, v T) {
	var z T
	if *lastErr != nil || any(v) == any(z) {
		return
	}

	*lastErr = v.Commit()
}

// IsDirty returns true if the record is non-nil and is dirty.
func IsDirty[T database.Record](v T) bool {
	var z T
	return any(v) != any(z) && v.IsDirty()
}

// Walk walks the record if it is non-nil, unless opts.Modified is set and the
// record is unmodified.
func Walk[T database.Record](lastErr *error, v T, opts database.WalkOptions, fn database.WalkFunc) {
	var z T
	if *lastErr != nil || any(v) == any(z) {
		return
	}
	if opts.Modified && !v.IsDirty() {
		return
	}

	*lastErr = v.Walk(opts, fn)
}

// WalkField walks an attribute. If opts.Modified is set, WalkField walks the
// record if it is non-nil and modified. Otherwise, WalkField will walk the
// existing record or create (but not retain) a new record and walk that.
func WalkField[T database.Record](lastErr *error, v T, make func() T, opts database.WalkOptions, fn database.WalkFunc) {
	var z T
	if !opts.Modified && any(v) == any(z) {
		// If the caller wants all records and the record is nil, create a new
		// record
		v = make()
	}

	Walk(lastErr, v, opts, fn)
}

// WalkField walks a parameterized attribute. If opts.Modified is set, WalkField
// iterates over the existing records, walking any that are modified. If
// opts.Modified is not set and getKeys is nil, WalkField does nothing.
// Otherwise, WalkField calls getKeys, iterates over the result, and walks all
// the corresponding records. If a record exists in the map, WalkField uses
// that; otherwise it creates (but does not retain) a new record and walks that.
func WalkMap[Record database.Record, Key keyType[MapKey], MapKey comparable](lastErr *error, m map[MapKey]Record, make func(Key) Record, getKeys func() ([]Key, error), opts database.WalkOptions, fn database.WalkFunc) {
	// If the caller only wants modified records, walking the map is sufficient
	if opts.Modified {
		for _, v := range m {
			Walk(lastErr, v, opts, fn)
		}
		return
	}

	// If the parent record does not provide a way to list keys, don't walk
	// anything. Otherwise the behavior of "walk all records" would depend on
	// which records had been previously loaded, which would be rather
	// unintuitive.
	if getKeys == nil {
		return
	}

	// Load the keys
	keys, err := getKeys()
	if err != nil {
		*lastErr = errors.UnknownError.Wrap(err)
		return
	}

	// Walk the key set using the existing record or creating a new one
	for _, k := range keys {
		v, ok := m[k.ForMap()]
		if !ok {
			v = make(k)
		}

		Walk(lastErr, v, opts, fn)
	}
}

func WalkComposite[T database.Record](v T, opts database.WalkOptions, fn database.WalkFunc) (skip bool, err error) {
	var z T
	if any(v) == any(z) {
		return true, nil
	}

	if opts.Modified && !v.IsDirty() {
		return true, nil
	}

	if opts.Values {
		return false, nil
	}

	return fn(v)
}
