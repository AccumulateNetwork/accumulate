package main

import (
	"context"
	"crypto/ed25519"
	"flag"
	"fmt"
	"log"
	"os"
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

// Start logging with dataset log
var dsl = logging.DataSetLog{}
var start time.Time

func main() {
	flag.Parse()

	parallelization := parallelism
	c := make(chan int)

	// Setup routine guard and limit
	guard := make(chan struct{}, maxGoroutines)

	// run clients in parallel
	wg := &sync.WaitGroup{}

	for ii := 0; ii <= parallelization; ii++ {
		guard <- struct{}{} // would block if guard channel is already filled
		wg.Add(parallelization)
		go func(c chan int) {
			for {
				v, more := <-c
				if !more {
					wg.Done()
					return
				}

				err := initClients(v, wg)
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
	dsl.DumpDataSetToDiskFile()
}

func init() {
	flag.StringVar(&serverUrl, "s", "http://127.0.1.1:26660/v2", "Accumulate server URL")
	flag.IntVar(&parallelism, "p", 5, "Number of parallel clients")
	flag.IntVar(&transactions, "t", 100, "Number of transactions per client")
}

// Init account creation and transaction sending
func initTxs(wg *sync.WaitGroup, ds *logging.DataSet, client *client.Client) error {
	defer wg.Done()
	// Setup routine guard and limit
	guard := make(chan struct{}, maxGoroutines)

	var m sync.Mutex
	deltas := make(map[int]float64)
	times := make(map[int]float64)
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
			t := time.Now()
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

			m.Lock()
			deltas[i] = time.Since(t).Seconds()
			m.Unlock()

			// time to release goroutine
			<-guard
		}(i)
		// stop timer
		<-timer.C
		times[i] = time.Since(start).Seconds()
	}

	for i := 0; i < transactions; i++ {
		ds.Save("tx_index", i, 10, true)
		ds.Save("time_since_start", times[i], 10, false)
		ds.Save("tx_time", deltas[i], 10, false)
	}

	//Stop time Tx executions
	log.Printf("The Txs execution took %v to run.\n", time.Since(start).Seconds())

	client.CloseIdleConnections()

	return nil
}

func DefaultOptions() {
	panic("unimplemented")
}

// Initiate several clients
func initClients(c int, wg *sync.WaitGroup) error {
	defer wg.Done()
	path := "load_tester"

	err := os.MkdirAll(path, 0600)
	if err != nil {
		fatalf("Error: creating dir: %v", err)
	}
	//don't remove the log after creating one.
	//defer os.RemoveAll(path)
	dsl.SetPath(path)
	dsl.SetProcessName("load")

	dataSets := []*logging.DataSet{}
	clients := []*client.Client{}

	//initialize the datasets and clients
	for i := 0; i <= c; i++ {
		dsName := fmt.Sprintf("client_%d", i)
		dsl.Initialize(dsName, logging.DefaultOptions())
		dataSets = append(dataSets, dsl.GetDataSet(dsName))

		client, err := client.New(serverUrl)
		checkf(err, "creating client")
		client.DebugRequest = false
		clients = append(clients, client)
	}

	// Initiate clients and wait for them to finish

	// Start the global clock
	start = time.Now()
	for i := 0; i <= c; i++ { // Create new client on localhost
		err = initTxs(wg, dataSets[i], clients[i])
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
