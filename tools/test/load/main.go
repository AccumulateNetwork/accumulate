package main

import (
	"context"
	"crypto/ed25519"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
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

var maxGoroutines = 25

// Start logging with dataset log
var dsl = logging.DataSetLog{}
var start time.Time

func main() {
	flag.Parse()

	//desired TPS is "transactions"
	transactionsPerClient := maxGoroutines
	numClientsPerBurst := transactions / transactionsPerClient

	duration := 10
	// run clients in parallel
	wg := &sync.WaitGroup{}

	clients, err := initializeClients(numClientsPerBurst * duration)
	checkf(err, "failed to initialize client %d", numClientsPerBurst*duration)

	// Start the global clock
	start = time.Now()

	for t := 0; t < duration; t++ {
		tick := time.Now()
		for ii := 0; ii <= numClientsPerBurst; ii++ {
			//	guard <- struct{}{} // would block if guard channel is already filled
			wg.Add(1)
			go func(v int) {
				err := initTxs(clients[v])
				if err != nil {
					fmt.Printf("Error: %v\n", err)
				}

				wg.Done()
			}(t*numClientsPerBurst + ii)
		}

		fmt.Printf("burst duration: %f\n", time.Since(tick).Seconds())
		time.Sleep(time.Second)
	}

	// force close channel
	wg.Wait()
	dsl.DumpDataSetToDiskFile()
}

func init() {
	flag.StringVar(&serverUrl, "s", "http://127.0.1.1:26660/v2", "Accumulate server URL")
	flag.IntVar(&parallelism, "p", 5, "Number of parallel clients")
	flag.IntVar(&transactions, "t", 100, "Number of transactions per client")
	flag.IntVar(&maxGoroutines, "r", 25, "Throttle go routines per client")
}

type Times struct {
	float64
}

// Init account creation and transaction sending
func initTxs(c *Client) error {
	// Setup routine guard and limit
	//guard := make(chan struct{}, maxGoroutines)

	var m sync.Mutex
	deltas := make([]float64, transactions)
	times := make([]float64, transactions)
	fmt.Println("starting ... ")

	txWaitGroup := sync.WaitGroup{}
	// run key generation in cycle
	for i := 0; i < maxGoroutines; i++ {
		// create accounts and store them
		acc, _ := createAccount(i)

		txWaitGroup.Add(1)
		// Add timer to measure TPS
		timer := time.NewTimer(time.Microsecond)

		// generate accounts and faucet in goroutines
		go func(n int) {
			defer txWaitGroup.Done()
			// time to release goroutine
			//		<-guard
			// start timer
			t := time.Now()

			timer.Reset(time.Microsecond)
			//fmt.Println("faucet")
			// faucet account and wait for Tx execution
			resp, err := c.Client.Faucet(context.Background(), &protocol.AcmeFaucet{Url: acc})
			if err != nil {
				log.Printf("Error: fauceting account with error: %v, on client %d, tx %d\n", err, c.Id, c.TxCount+1)
				return
			}
			//fmt.Printf("faucet done")
			txReq := api.TxnQuery{}
			txReq.Txid = resp.TransactionHash
			txReq.Wait = time.Second * 10
			txReq.IgnorePending = false

			_, err = c.Client.QueryTx(context.Background(), &txReq)
			if err != nil {
				return
			}

			m.Lock()
			deltas[n] = time.Since(t).Seconds()
			c.TxCount++
			m.Unlock()
		}(i)

		<-timer.C

		//capture the timestamp of when the transaction started
		times[i] = time.Since(start).Seconds()
	}

	txWaitGroup.Wait()

	c.Client.CloseIdleConnections()

	//sort by times
	sort.Slice(deltas, func(i, j int) bool {
		return times[i] < times[j]
	})
	sort.Slice(times, func(i, j int) bool {
		return times[i] < times[j]
	})

	for i := 0; i < transactions; i++ {
		c.DataSet.Lock()
		c.DataSet.Save("index", i, 10, true)
		c.DataSet.Save("clientId", c.Id, 10, false)
		c.DataSet.Save("time_since_start", times[i], 10, false)
		c.DataSet.Save("tx_time", deltas[i], 10, false)
		c.DataSet.Unlock()
	}

	//Stop time Tx executions
	log.Printf("The Txs execution took %v to run.\n", time.Since(start).Seconds())

	return nil
}

func DefaultOptions() {
	panic("unimplemented")
}

//Client
type Client struct {
	DataSet *logging.DataSet
	Client  *client.Client
	Id      int
	TxCount int
}

// initializeClients the clinet
func initializeClients(c int) ([]*Client, error) {
	path := "load_tester"
	err := os.MkdirAll(path, 0755)
	if err != nil {
		fatalf("Error: creating dir: %v", err)
	}
	dsl.SetPath(path)
	dsl.SetProcessName("load")

	clients := []*Client{}

	//initialize the datasets and clients
	for i := 0; i <= c; i++ {
		dsName := fmt.Sprintf("client_%d", i)
		dsl.Initialize(dsName, logging.DefaultOptions())
		ds := dsl.GetDataSet(dsName)

		client, err := client.New(serverUrl)
		checkf(err, "creating client")
		client.DebugRequest = false
		clients = append(clients, &Client{ds, client, i + 1, 0})
	}

	return clients, nil
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
