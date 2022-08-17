package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"

	"gitlab.com/accumulatenetwork/accumulate/tools/internal/factom-genesis"
)

func main() {
	go func() {
		err := http.ListenAndServe(":6060", nil)
		log.Fatalf("pprof listener failed: %v", err)
	}()

	factom.InitSim()
	err := factom.FaucetWithCredits()
	if err != nil {
		log.Fatalf("failed to faucet account %v", err)
	}

	factom.Process()
	factom.CreateAccumulateSnapshot()

}
