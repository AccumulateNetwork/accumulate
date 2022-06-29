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
var transactions int
var duration int
var maxGoroutines = 25

// Start logging with dataset log
var dsl = logging.DataSetLog{}
var start time.Time

func main() {
	flag.Parse()

	//desired TPS is "transactions"
	transactionsPerClient := maxGoroutines
	numClientsPerBurst := transactions / transactionsPerClient

	maxNumClients := numClientsPerBurst * duration

	totalTransactions := maxNumClients * transactionsPerClient
	// run clients in parallel
	wg := &sync.WaitGroup{}

	clients, err := initializeClients(maxNumClients)
	checkf(err, "failed to initialize client %d", maxNumClients)

	// Start the global clock
	start = time.Now()

	for simTime := 0; simTime < duration; simTime++ {
		tick := time.Now()
		for ii := 0; ii < numClientsPerBurst; ii++ {
			//	guard <- struct{}{} // would block if guard channel is already filled
			wg.Add(1)
			go func(simTime float64, v int) {
				defer wg.Done()
				err := initTxs(simTime, transactionsPerClient, clients[v])
				if err != nil {
					fmt.Printf("Error: %v\n", err)
				}
			}(float64(simTime), simTime*numClientsPerBurst+ii)
		}
		time.Sleep(time.Second - time.Since(tick))
	}

	// force close channel
	wg.Wait()
	txCount := 0
	for j := 0; j < maxNumClients; j++ {
		txCount += clients[j].TxCount
	}
	txPassed := txCount
	txFailed := totalTransactions - txPassed

	header := fmt.Sprintf("## Runtime %d seconds with transaction load per second: %d\n", duration, transactions)
	header += fmt.Sprintf("## Total Tx : %d, Tx Passed : %d, Tx Failed : %d\n", txCount, txPassed, txFailed)
	dsl.SetHeader(header)

	_, err = dsl.DumpDataSetToDiskFile()
	if err != nil {
		checkf(err, "cannot dump data set to disk")
	}
}

func init() {
	flag.StringVar(&serverUrl, "s", "http://127.0.1.1:26660/v2", "Accumulate server URL")
	flag.IntVar(&transactions, "t", 100, "Number of transactions per second")
	flag.IntVar(&maxGoroutines, "r", 25, "Number of transactions per client (i.e. go routines)")
	flag.IntVar(&duration, "d", 5, "Throttle go routines per client")
}

// Init account creation and transaction sending
func initTxs(simTime float64, transactionsPerClient int, c *Client) error {
	var m sync.Mutex
	deltas := make([]float64, transactions)
	times := make([]float64, transactions)

	txWaitGroup := sync.WaitGroup{}
	// run key generation in cycle
	for i := 0; i < transactionsPerClient; i++ {
		// create accounts and store them
		acc, _ := createAccount(i)

		txWaitGroup.Add(1)

		// generate accounts and faucet in goroutines
		go func(n int) {
			defer txWaitGroup.Done()
			// start timer
			t := time.Now()

			// faucet account and wait for Tx execution
			resp, err := c.Client.Faucet(context.Background(), &protocol.AcmeFaucet{Url: acc})
			if err != nil {
				log.Printf("Error: fauceting account with error: %v, on client %d, tx %d\n", err, c.Id, c.TxCount+1)
				return
			}
			txReq := api.TxnQuery{}
			txReq.Txid = resp.TransactionHash
			txReq.Wait = time.Second * 100
			txReq.IgnorePending = false

			_, err = c.Client.QueryTx(context.Background(), &txReq)
			if err != nil {
				log.Printf("Error: waiting for transaction to complete account with error: %v, on client %d, tx %d\n", err, c.Id, c.TxCount+1)
				return
			}

			m.Lock()
			deltas[n] = time.Since(t).Seconds()
			c.TxCount++
			m.Unlock()
		}(i)

		//capture the timestamp of when the transaction started
		times[i] = time.Since(start).Seconds()
	}

	txWaitGroup.Wait()

	c.Client.CloseIdleConnections()

	for i := 0; i < transactionsPerClient; i++ {
		c.DataSet.Lock()
		c.DataSet.Save("index", (transactionsPerClient*c.Id)+i, 10, true)
		c.DataSet.Save("simTime", simTime, 10, false)
		c.DataSet.Save("clientId", c.Id, 10, false)
		c.DataSet.Save("settlementTime", deltas[i], 10, false)
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
	dsName := fmt.Sprintf("settlemint_time")
	dsl.Initialize(dsName, logging.DefaultOptions())
	ds := dsl.GetDataSet(dsName)

	clients := []*Client{}

	//initialize the datasets and clients
	for i := 0; i <= c; i++ {
		client, err := client.New(serverUrl)
		checkf(err, "creating client")
		client.DebugRequest = false
		clients = append(clients, &Client{ds, client, i, 0})
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
