package main

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	acctesting "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/AccumulateNetwork/accumulated/networks"
	"github.com/spf13/cobra"
	rpc "github.com/tendermint/tendermint/rpc/client/http"
)

var cmdLoadTest = &cobra.Command{
	Use: "loadtest",
	Run: loadTest,
}

var flagLoadTest struct {
	Networks         []string
	Remotes          []string
	WalletCount      int
	TransactionCount int
	BatchSize        int
	BatchDelay       time.Duration
}

func init() {
	cmdMain.AddCommand(cmdLoadTest)

	cmdLoadTest.Flags().StringSliceVarP(&flagLoadTest.Networks, "network", "n", nil, "Network to load test (name or number)")
	cmdLoadTest.Flags().StringSliceVarP(&flagLoadTest.Remotes, "remote", "r", nil, "Node to load test, e.g. tcp://1.2.3.4:5678")
	cmdLoadTest.Flags().IntVar(&flagLoadTest.WalletCount, "wallets", 100, "Number of generated recipient wallets")
	cmdLoadTest.Flags().IntVar(&flagLoadTest.TransactionCount, "transactions", 1000, "Number of generated transactions")
	// cmdLoadTest.Flags().IntVar(&flagLoadTest.BatchSize, "batches", 0, "Transaction batch size; defaults to 1/5 of the transaction count")
	// cmdLoadTest.Flags().DurationVarP(&flagLoadTest.BatchDelay, "batch-delay", "d", time.Second/5, "Delay after each batch")
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
	for _, name := range flagLoadTest.Networks {
		net := networks.Networks[name]
		if net == nil {
			fmt.Fprintf(os.Stderr, "Error: unknown network %q\n", flagInit.Net)
			os.Exit(1)
		}

		lAddr := fmt.Sprintf("tcp://%s:%d", net.Nodes[0].IP, net.Port+node.TmRpcPortOffset)
		client, err := rpc.New(lAddr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to create RPC client for network %q: %v\n", name, err)
			os.Exit(1)
		}

		clients = append(clients, client)
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

		client, err := rpc.New(r)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to create RPC client for remote %s: %v\n", r, err)
			os.Exit(1)
		}

		clients = append(clients, client)
	}

	if len(clients) == 0 {
		fmt.Fprintf(os.Stderr, "Error: at least one --network or --remote is required\n")
		cmd.Usage()
		os.Exit(1)
	}

	relay := relay.New(clients...)
	query := api.NewQuery(relay)

	_, privateKeySponsor, _ := ed25519.GenerateKey(nil)

	addrList, err := acctesting.RunLoadTest(query, &privateKeySponsor, flagLoadTest.WalletCount, flagLoadTest.TransactionCount)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	time.Sleep(10000 * time.Millisecond)

	for _, v := range addrList[1:] {
		resp, err := query.GetChainState(&v, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		output, err := json.Marshal(resp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("%s : %s\n", v, string(output))
	}
}
