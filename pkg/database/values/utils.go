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
	if opts.Changes && !v.IsDirty() {
		return
	}

	*lastErr = v.Walk(opts, fn)
}

func WalkComposite[T database.Record](v T, opts database.WalkOptions, fn database.WalkFunc) (skip bool, err error) {
	var z T
	if any(v) == any(z) {
		return true, nil
	}

	if opts.Changes && !v.IsDirty() {
		return true, nil
	}

	if opts.Values {
		return false, nil
	}

	return fn(v)
}
