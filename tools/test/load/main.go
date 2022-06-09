package main

import (
	"context"
	"crypto/ed25519"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var serverUrl string

func main() {
	flag.StringVar(&serverUrl, "s", "http://127.0.1.1:26660/v2", "Accumulate server URL")
	flag.Parse()

	// Initiate clients and wait for them to finish
	err := initClients(3)
	if err != nil {
		os.Exit(1)
	}
}

// Init new client from server URL input using client.go
func initClient(server string) error {
	// Create new client on localhost
	client, err := client.New(server)
	checkf(err, "creating client")
	client.DebugRequest = true

	// Limit amount of goroutines
	maxGoroutines := 25
	guard := make(chan struct{}, maxGoroutines)

	file, err := os.OpenFile("logs.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}

	log.SetOutput(file)
	// Start time
	start := time.Now()

	// run key generation in cycle
	for i := 0; i < 50; i++ {
		// create accounts and store them
		acc, _ := createAccount(i)

		// log the time when the account was created
		log.Printf("Account %d created at %s\n", i, time.Now().Format(time.RFC3339))

		guard <- struct{}{} // would block if guard channel is already filled

		// Add timer to measure TPS
		timer := time.NewTimer(time.Microsecond)

		// generate accounts and faucet in goroutines
		go func(n int) {
			// start timer
			timer.Reset(time.Microsecond)

			// faucet account and wait for Tx execution
			resp, err := client.Faucet(context.Background(), &protocol.AcmeFaucet{Url: acc})
			if err != nil {
				fmt.Printf("Error: faucet: %v\n", err)
			}

			txReq := api.TxnQuery{}
			txReq.Txid = resp.TransactionHash
			txReq.Wait = time.Second * 100
			txReq.IgnorePending = false

			_, err = client.QueryTx(context.Background(), &txReq)
			if err != nil {
				return
			}
			// wait for timer to fire
			log.Printf("Execution time %s\n", time.Since(<-timer.C))

			// time to release goroutine
			<-guard
		}(i)
		// stop timer
		<-timer.C
	}

	// wait for goroutines to finish
	for i := 0; i < maxGoroutines; i++ {
		guard <- struct{}{}
	}

	//Stop time Tx executions
	stop := time.Now()
	fmt.Printf("The Txs execution took %v to run.\n", stop.Sub(start))

	return nil
}

// Initiate several clients
func initClients(c int) error {
	// TODO run multiple clients in parallel
	for i := 0; i < c; i++ {
		err := initClient(serverUrl)
		if err != nil {
			return err
		}
	}
	return nil
}

// helper function to generate key and create account and return the address
func createAccount(i int) (*url.URL, error) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		fmt.Printf("Error: generating keys: %v\n", err)
	}

	acc, err := protocol.LiteTokenAddress(pub, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		fmt.Printf("Error: creating Lite Token account: %v\n", err)
	}

	fmt.Printf("Account %d: %s\n", i, acc)

	return acc, nil
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func checkf(err error, format string, otherArgs ...interface{}) {
	if err != nil {
		fatalf(format+": %v", append(otherArgs, err)...)
	}
}
