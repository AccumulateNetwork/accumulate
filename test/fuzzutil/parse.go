package fuzzutil

import (
	"encoding"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func AddValue(f *testing.F, v encoding.BinaryMarshaler) {
	data, err := v.MarshalBinary()
	require.NoError(f, err)
	f.Add(data)
}

func MustParseUrl(t testing.TB, v string) *url.URL {
	if v == "" {
		return nil
	}
	u, err := url.Parse(v)
	if err != nil {
		t.Skip()
	}
	return u
}

func MustParseHash(t testing.TB, v []byte) [32]byte {
	if len(v) != 32 {
		t.Skip()
	}
	return *(*[32]byte)(v)
}

func MustParseBigInt(t testing.TB, v string) big.Int {
	u := new(big.Int)
	u, ok := u.SetString(v, 10)
	if !ok {
		t.Skip()
	}
	return *u
}
