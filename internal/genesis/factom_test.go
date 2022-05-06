package genesis

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFactomAddressesUpload(t *testing.T) {
	value, err := LoadFactomAddressesAndBalances("test_factom_addresses")
	require.NoError(t, err)
	for _, v := range value {
		fmt.Print("Address : ", v.Address)
		fmt.Println("Balance : ", v.Balance)
	}
}
