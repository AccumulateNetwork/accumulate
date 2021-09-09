package refactor

import (
	"fmt"
	"strings"
)

type Amount int64

func (a Amount) String() string {
	left := a / 100000000
	right := a - left*100000000
	str := fmt.Sprintf("%d.%08d", left, right)
	return strings.Trim(strings.Trim(str, "0"), ".")
}
