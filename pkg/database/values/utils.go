// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package values

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
)

func GetOrCreate[T any, R any](ptr *T, create func(R) T, a0 R) T {
	var z T
	if any(*ptr) != any(z) {
		return *ptr
	}

	*ptr = create(a0)
	return *ptr
}

func GetOrCreateMap[T any, K comparable, A0 any](ptr *map[K]T, key K, create func(A0) T, a0 A0) T {
	if *ptr == nil {
		*ptr = map[K]T{}
	}

	if v, ok := (*ptr)[key]; ok {
		return v
	}

	v := create(a0)
	(*ptr)[key] = v
	return v
}

func GetOrCreateMap1[T any, K comparable, A0, A1 any](ptr *map[K]T, key K, create func(A0, A1) T, a0 A0, a1 A1) T {
	if *ptr == nil {
		*ptr = map[K]T{}
	}

	if v, ok := (*ptr)[key]; ok {
		return v
	}

	v := create(a0, a1)
	(*ptr)[key] = v
	return v
}

func Commit[T database.Record](lastErr *error, v T) {
	var z T
	if *lastErr != nil || any(v) == any(z) {
		return
	}

	*lastErr = v.Commit()
}

func IsDirty[T database.Record](v T) bool {
	var z T
	return any(v) != any(z) && v.IsDirty()
}

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

func WalkField[T database.Record](lastErr *error, v T, make func() T, opts database.WalkOptions, fn database.WalkFunc) {
	var z T
	if !opts.Modified && any(v) == any(z) {
		// If the caller wants all records and the record is nil, create a new
		// record
		v = make()
	}

	Walk(lastErr, v, opts, fn)
}

func WalkMap[T database.Record, K comparable](lastErr *error, m map[K]T, make func(K) T, keys func() []K, opts database.WalkOptions, fn database.WalkFunc) {
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
	if keys == nil {
		return
	}

	for _, k := range keys() {
		// Check for an existing record
		v, ok := m[k]
		if !ok {
			// Create a new record
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
