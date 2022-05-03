package main

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/factom"
)

func main() {
	fmt.Println("-----------------")
	value, err := factom.LoadFactomAddressesAndBalances("../balances-341839")
	if err != nil {
		fmt.Println(err.Error())
	}
	for _, v := range value {
		fmt.Print("Address : ", v.Address)
		fmt.Println("Balance : ", v.Balance)
	}
}
