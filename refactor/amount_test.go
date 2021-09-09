package refactor

import (
	"strconv"
	"testing"
)

func TestAmount(t *testing.T) {
	values := []string{"7.5", "10.0004", "1000"}

	for _, v := range values {
		f, _ := strconv.ParseFloat(v, 64)
		var t7p5 = Amount(f * 100000000)
		s := t7p5.String()
		if s != v {
			t.Errorf("expected %s got %s", v, s)
		}
	}
}
