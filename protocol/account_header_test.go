package protocol

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

func TestStateHeader(t *testing.T) {
	u, err := url.Parse("acme/chain/path")
	require.NoError(t, err)
	header := AccountHeader{Url: u}

	data, err := header.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	header2 := AccountHeader{}

	err = header2.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

	if header.Url != header2.Url {
		t.Fatalf("header adi chain path doesnt match")
	}

}
