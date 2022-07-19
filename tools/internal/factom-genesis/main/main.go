package main

import (
	"log"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/tools/internal/factom-genesis"
)

func main() {
	err := factom.SetPrivateKeyAndOrigin("priv_validator_key.json")
	if err != nil {
		log.Fatalf("Error : %v", err)
	}
	factom.InitSim()
	err = factom.FaucetWithCredits()
	if err != nil {
		log.Fatalf("failed to faucet account %v", err)
	}

	factom.Process()
	time.Sleep(10 * time.Second)
	factom.CreateAccumulateSnapshot()

}
