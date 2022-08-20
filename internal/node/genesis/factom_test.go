package genesis

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	testdata "gitlab.com/accumulatenetwork/accumulate/test/data"
)

func TestFactomAddressesUpload(t *testing.T) {
	value, err := LoadFactomAddressesAndBalances(strings.NewReader(testdata.FactomAddresses))
	require.NoError(t, err)
	for _, v := range value {
		fmt.Print("Address : ", v.Address)
		fmt.Println("Balance : ", v.Balance)
	}
}
