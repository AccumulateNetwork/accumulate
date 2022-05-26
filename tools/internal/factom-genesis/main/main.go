package main

import (
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/factom-genesis"
)

const (
	Key_Name         = "keytest-0-0"
	Key_Private_Key  = "d125672c7f0af6fd82c87c884560c6fcbaf03bcd51ac578057369d7e99274f3c"
	Key_Public_Key   = "f40e4b1b3bf80938c4e9a541f395af8fd8a3ef39ea779b3bd85ce65fb17deb72"
	OriginAccountUrl = "acc://c1dd0c5b8540f64be83eb5704678d3556829498f3726b7d8/ACME"
)

var faucet = true

func main() {
	url, _ := factom.AccountFromPrivateKey(Key_Private_Key, Key_Public_Key)
	fmt.Println("URL : ", url)
	if faucet {
		factom.FaucetWithCredits(factom.LOCAL_URL)
	}
	entries := factom.CurlEntryFromFactom()
	factom.GetDataAndPopulateQueue(entries)
	factom.WriteDataFromQueueToAccumulate()
}
