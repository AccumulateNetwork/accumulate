package genesis

import (
	"fmt"
	"testing"
)

func TestFactomAddressesUpload(t *testing.T) {
	value, err := LoadFactomAddressesAndBalances("test_factom_addresses")
	if err != nil {
		fmt.Println(err.Error())
	}
	for _, v := range value {
		fmt.Print("Address : ", v.Address)
		fmt.Println("Balance : ", v.Balance)
	}
}
