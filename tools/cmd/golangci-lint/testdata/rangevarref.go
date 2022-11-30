// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pkg

func rangevarref_map_slice() {
	var values [][]int
	for k := range map[[1]int]int{{1}: 1, {2}: 2, {3}: 3} {
		values = append(values, k[:]) // want `Taking the address of a range variable is unsafe`
	}
}

func rangevarref_value_addr() {
	var values []*int
	for _, x := range []int{1, 2, 3} {
		values = append(values, &x) // want `Taking the address of a range variable is unsafe`
	}
}

func rangevarref_array_slice() {
	var values [][]int
	for _, x := range [][1]int{{1}, {2}, {3}} {
		values = append(values, x[:]) // want `Taking the address of a range variable is unsafe`
	}
}

func rangevarref_value_addr_safe() {
	var values []*int
	for _, x := range []int{1, 2, 3} {
		x := x
		values = append(values, &x)
	}
}

func rangevarref_array_slice_safe() {
	var values [][]int
	for _, x := range [][1]int{{1}, {2}, {3}} {
		x := x
		values = append(values, x[:])
	}
}

func rangevarref_map_slice_safe() {
	var values [][]int
	for k := range map[[1]int]int{{1}: 1, {2}: 2, {3}: 3} {
		k := k
		values = append(values, k[:])
	}
}

func rangevarref_slice_index() {
	for i := range []int{1, 2, 3} {
		println(i)
	}
}

func rangevarref_string_addr() {
	var values []*string
	for _, s := range []string{"11", "22", "33"} {
		values = append(values, &s) // want `Taking the address of a range variable is unsafe`
	}
}

func rangevarref_string_slice() {
	var values []string
	for _, s := range []string{"11", "22", "33"} {
		values = append(values, s[:1])
	}
}

func rangevarref_reftype_addr() {
	type Type struct{ X int }
	var values []**Type
	for _, v := range []*Type{&Type{1}, &Type{2}, &Type{3}} {
		values = append(values, &v) // want `Taking the address of a range variable is unsafe`
	}
}
