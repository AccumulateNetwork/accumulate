// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package values

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
)

func GetOrCreate[T any](ptr *T, create func() T) T {
	var z T
	if any(*ptr) != any(z) {
		return *ptr
	}

	*ptr = create()
	return *ptr
}

func GetOrCreateMap[T any, K comparable](ptr *map[K]T, key K, create func() T) T {
	if *ptr == nil {
		*ptr = map[K]T{}
	}

	if v, ok := (*ptr)[key]; ok {
		return v
	}

	v := create()
	(*ptr)[key] = v
	return v
}

func GetOrCreateMap1[T any, K comparable, A1 any](ptr *map[K]T, key K, create func(A1) T, a1 A1) T {
	if *ptr == nil {
		*ptr = map[K]T{}
	}

	if v, ok := (*ptr)[key]; ok {
		return v
	}

	v := create(a1)
	(*ptr)[key] = v
	return v
}

func GetOrCreateMap2[T any, K comparable, A1, A2 any](ptr *map[K]T, key K, create func(A1, A2) T, a1 A1, a2 A2) T {
	if *ptr == nil {
		*ptr = map[K]T{}
	}

	if v, ok := (*ptr)[key]; ok {
		return v
	}

	v := create(a1, a2)
	(*ptr)[key] = v
	return v
}

func GetOrCreateMap3[T any, K comparable, A1, A2, A3 any](ptr *map[K]T, key K, create func(A1, A2, A3) T, a1 A1, a2 A2, a3 A3) T {
	if *ptr == nil {
		*ptr = map[K]T{}
	}

	if v, ok := (*ptr)[key]; ok {
		return v
	}

	v := create(a1, a2, a3)
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

func WalkField[T database.Record](lastErr *error, v T, get func() T, opts database.WalkOptions, fn database.WalkFunc) {
	// If the caller wants all records,
	var z T
	if !opts.Modified && any(v) == any(z) {
		v = get()
	}

	Walk(lastErr, v, opts, fn)
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
