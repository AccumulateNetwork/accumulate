package encoding_test

import (
	"io"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

func TestWriter_WriteBigInt(t *testing.T) {
	w := NewWriter(io.Discard)

	w.WriteBigInt(1, big.NewInt(-1))
	_, _, err := w.Reset(nil)
	require.Error(t, err)
}
