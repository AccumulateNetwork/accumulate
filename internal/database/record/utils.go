// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package record

func FieldGetOrCreate[T any](ptr *T, create func() T) T {
	var z T
	if any(*ptr) != any(z) {
		return *ptr
	}

	*ptr = create()
	return *ptr
}

func FieldGetOrCreateMap[T any, K comparable](ptr *map[K]T, key K, create func() T) T {
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

func FieldCommit[T Record](lastErr *error, field T) {
	var z T
	if *lastErr != nil || any(field) == any(z) {
		return
	}

	*lastErr = field.Commit()
}

func FieldIsDirty[T Record](field T) bool {
	var z T
	return any(field) != any(z) && field.IsDirty()
}

func FieldWalkChanges[T Record](lastErr *error, field T, fn WalkFunc) {
	var z T
	if *lastErr != nil || any(field) == any(z) {
		return
	}

	*lastErr = field.WalkChanges(fn)
}
