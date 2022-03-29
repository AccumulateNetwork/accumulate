package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatAmount(t *testing.T) {
	cases := []struct {
		Amount    uint64
		Precision int
		Result    string
	}{
		{123, 0, "123"},
		{123, 1, "12.3"},
		{123, 2, "1.23"},
		{123, 3, "0.123"},
		{123, 4, "0.0123"},
	}

	for _, c := range cases {
		assert.Equal(t, c.Result, FormatAmount(c.Amount, c.Precision))
	}
}
