package main

import (
	"encoding/hex"
	"fmt"
	"log"

	f2 "github.com/FactomProject/factom"
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
	pk, err := hex.DecodeString(Key_Private_Key)
	if err != nil {
		log.Fatalf("invalid private key %v", err)
	}
	url, _ := factom.AccountFromPrivateKey(pk)
	fmt.Println("URL : ", url)
	if faucet {
		err := factom.FaucetWithCredits(factom.LOCAL_URL)
		if err != nil {
			log.Fatalf("cannot faucet account %v", err)
		}
	}

	f2.SetFactomdServer("https://api.factomd.net")
	// f2.SetFactomdServer("http://localhost:8088")

	entries := factom.EntriesFromFactom()
	factom.GetDataAndPopulateQueue(entries)
	factom.WriteDataFromQueueToAccumulate(factom.LOCAL_URL)
}
