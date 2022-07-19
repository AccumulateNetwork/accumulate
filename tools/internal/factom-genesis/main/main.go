package main

import (
	"log"

	"gitlab.com/accumulatenetwork/accumulate/tools/internal/factom-genesis"
)

func main() {

	factom.InitSim()
	err := factom.FaucetWithCredits()
	if err != nil {
		log.Fatalf("failed to faucet account %v", err)
	}

	factom.Process()
	factom.CreateAccumulateSnapshot()

}
