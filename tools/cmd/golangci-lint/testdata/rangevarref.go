package pkg

func rangevarref_value_addr() {
	var values []*int
	for _, x := range []int{1, 2, 3} {
		values = append(values, &x) // want `Taking the address of a value-type range variable is unsafe`
	}
}

func rangevarref_array_slice() {
	var values [][]int
	for _, x := range [][1]int{{1}, {2}, {3}} {
		values = append(values, x[:]) // want `Taking the address of a value-type range variable is unsafe`
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
