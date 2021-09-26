package main

import (
	"crypto/ed25519"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/AccumulateNetwork/accumulated/internal/relay"
	"github.com/AccumulateNetwork/accumulated/networks"
	"github.com/AccumulateNetwork/accumulated/router"
	"github.com/AccumulateNetwork/accumulated/testing"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/spf13/cobra"
	rpc "github.com/tendermint/tendermint/rpc/client/http"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
)

var cmdLoadTest = &cobra.Command{
	Use: "loadtest",
	Run: loadTest,
}

var flagLoadTest struct {
	Networks         []int
	Remotes          []string
	WalletCount      int
	TransactionCount int
	BatchSize        int
	BatchDelay       time.Duration
}

func init() {
	cmdMain.AddCommand(cmdLoadTest)

	cmdLoadTest.Flags().IntSliceVarP(&flagLoadTest.Networks, "network", "n", nil, "Network to load test")
	cmdLoadTest.Flags().StringSliceVarP(&flagLoadTest.Remotes, "remote", "r", nil, "Node to load test, e.g. tcp://1.2.3.4:5678")
	cmdLoadTest.Flags().IntVar(&flagLoadTest.WalletCount, "wallets", 100, "Number of generated recipient wallets")
	cmdLoadTest.Flags().IntVar(&flagLoadTest.TransactionCount, "transactions", 1000, "Number of generated transactions")
	cmdLoadTest.Flags().IntVar(&flagLoadTest.BatchSize, "batches", 0, "Transaction batch size; defaults to 1/5 of the transaction count")
	cmdLoadTest.Flags().DurationVarP(&flagLoadTest.BatchDelay, "delay", "d", time.Second/5, "Delay after each batch")
}

func loadTest(cmd *cobra.Command, args []string) {
	var clients []*rpc.HTTP

	if flagLoadTest.BatchSize < 1 {
		if flagLoadTest.TransactionCount > 5 {
			flagLoadTest.BatchSize = flagLoadTest.TransactionCount / 5
		} else {
			flagLoadTest.BatchSize = 1
		}
	}

	// Create clients for networks
	for _, n := range flagLoadTest.Networks {
		net := networks.Networks[n]
		lAddr := fmt.Sprintf("tcp://%s:%d", net.Ip[0], net.Port+1)
		client, err := rpc.New(lAddr, "/websocket")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to create RPC client for network %d: %v\n", n, err)
			os.Exit(1)
		}

		clients = append(clients, client)
	}

	if len(clients) == 0 {
		fmt.Fprintf(os.Stderr, "Error: at least one --network or --remote is required\n")
		cmd.Usage()
		os.Exit(1)
	}

	// Create clients for remotes
	for _, r := range flagLoadTest.Remotes {
		u, err := url.Parse(r)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: not a valid URL: %s: %v\n", r, err)
			os.Exit(1)
		}

		if u.Path != "" && u.Path != "/" {
			fmt.Fprintf(os.Stderr, "Error: remote URL must not contain a path: %s\n", r)
			os.Exit(1)
		}

		client, err := rpc.New(r, "/websocket")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to create RPC client for remote %s: %v\n", r, err)
			os.Exit(1)
		}

		clients = append(clients, client)
	}

	_, sponsor, _ := ed25519.GenerateKey(nil)
	_, recipient, _ := ed25519.GenerateKey(nil)
	gtx, err := testing.NewAcmeWalletTx(sponsor, recipient, []byte("fake txid"), "dc/ACME")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create initial TX: %v\n", err)
		os.Exit(1)
	}

	relay := relay.New(clients...)
	_, err = relay.SendTx(gtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to submit initial TX: %v\n", err)
		os.Exit(1)
	}

	var c int
	count, err := testing.Load(func(gt *transactions.GenTransaction) (*coretypes.ResultBroadcastTx, error) {
		c++
		if c%flagLoadTest.BatchSize == 0 {
			fmt.Printf("Sending batch %d\n", c/flagLoadTest.BatchSize)
			relay.BatchSend()
			time.Sleep(flagLoadTest.BatchDelay)
		}
		return relay.BatchTx(gtx)
	}, recipient, flagLoadTest.WalletCount, flagLoadTest.TransactionCount)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: load test failed: %v\n", err)
		os.Exit(1)
	}

	c++
	fmt.Printf("Sending batch %d\n", c/flagLoadTest.BatchSize)
	relay.BatchSend()

	time.Sleep(10000 * time.Millisecond)

	for addr := range count {
		query := router.NewQuery(relay)
		queryTokenUrl := addr + "/dc/ACME"
		resp, err := query.GetChainState(&queryTokenUrl, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to query %s: %v\n", addr, err)
			os.Exit(1)
		}
		fmt.Printf("%s => %#v\n", addr, resp)
	}
}
