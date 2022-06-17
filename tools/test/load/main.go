package main

import (
	"context"
	"crypto/ed25519"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var serverUrl string
var parallelism, transactions int

const maxGoroutines = 25

func main() {
	flag.Parse()

	parallelization := parallelism
	c := make(chan int)

	// Setup routine guard and limit
	guard := make(chan struct{}, maxGoroutines)

	// run clients in parallel
	var wg sync.WaitGroup
	wg.Add(parallelization)
	for ii := 0; ii <= parallelization; ii++ {
		guard <- struct{}{} // would block if guard channel is already filled
		go func(c chan int) {
			for {
				v, more := <-c
				if !more {
					wg.Done()
					return
				}

				err := initClients(v)
				if err != nil {
					fmt.Printf("Error: %v\n", err)
				}
				// time to release goroutine
				<-guard
			}
		}(c)
	}

	// send number of clients to be created
	for ii := 0; ii <= parallelization; ii++ {
		c <- ii
	}

	// force close channel
	close(c)
	wg.Wait()
}

func init() {
	flag.StringVar(&serverUrl, "s", "http://127.0.1.1:26660/v2", "Accumulate server URL")
	flag.IntVar(&parallelism, "p", 5, "Number of parallel clients")
	flag.IntVar(&transactions, "t", 100, "Number of transactions per client")
}

// Init new client from server URL input using client.go
func initClient(server string) error {
	// Create new client on localhost
	client, err := client.New(server)
	checkf(err, "creating client")
	client.DebugRequest = false

	// Setup routine guard and limit
	guard := make(chan struct{}, maxGoroutines)

	// Start time
	start := time.Now()

	// run key generation in cycle
	for i := 0; i < transactions; i++ {
		// create accounts and store them
		acc, _ := createAccount(i)

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
				log.Printf("Error: fauceting account: %v\n", err)
			}

			txReq := api.TxnQuery{}
			txReq.Txid = resp.TransactionHash
			txReq.Wait = time.Second * 10
			txReq.IgnorePending = false

			_, err = client.QueryTx(context.Background(), &txReq)
			if err != nil {
				return
			}

			// time to release goroutine
			<-guard
		}(i)
		// stop timer
		<-timer.C
	}

	//Stop time Tx executions
	stop := time.Now()
	log.Printf("The Txs execution took %v to run.\n", stop.Sub(start))

	client.CloseIdleConnections()

	return nil
}

func DefaultOptions() {
	panic("unimplemented")
}

// Initiate several clients
func initClients(c int) error {
	// Initiate clients and wait for them to finish
	for i := 0; i <= c; i++ {
		err := initClient(serverUrl)
		if err != nil {
			return err
		}
	}

	// Start logging with dataset log
	dsl := logging.DataSetLog{}
	path, err := ioutil.TempDir("", "DataSetTest")
	if err != nil {
		fatalf("Error: creating temp dir: %v", err)
	}

	err = os.MkdirAll(path, 0600)
	if err != nil {
		fatalf("Error: creating dir: %v", err)
	}
	defer os.RemoveAll(path)
	dsl.SetPath(path)
	dsl.SetProcessName("main")

	dsl.Initialize("creating client", logging.DefaultOptions())

	ds := dsl.GetDataSet("creating client")

	for ii := 0; ii < parallelism; ii++ {
		clients := "client" + strconv.Itoa(ii+1)
		ds.Lock().Save("int_client", ii, 32, true).
			Save("client", clients, 32, false).Unlock()
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
