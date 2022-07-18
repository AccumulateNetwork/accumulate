package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestParseAmount(t *testing.T) {
	cases := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}

	for i, c := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			const precision = 18
			str := "10000000." + strings.Repeat(c, precision)
			v, err := parseAmount(str, precision)
			require.NoError(t, err)
			require.Equal(t, str, protocol.FormatBigAmount(v, precision))
		})
	}
}
