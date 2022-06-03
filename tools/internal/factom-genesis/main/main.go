package main

import (
	"encoding/hex"
	"log"

	f2 "github.com/FactomProject/factom"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/factom-genesis"
)

const (
	Key_Private_Key = "d125672c7f0af6fd82c87c884560c6fcbaf03bcd51ac578057369d7e99274f3c"
)

var faucet = true

func main() {
	pk, err := hex.DecodeString(Key_Private_Key)
	if err != nil {
		log.Fatalf("invalid private key %v", err)
	}
	url, _ := factom.AccountFromPrivateKey(pk)
	log.Println("URL : ", url)
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
