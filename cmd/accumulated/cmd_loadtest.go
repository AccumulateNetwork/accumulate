package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/AccumulateNetwork/accumulate/internal/api"
	"github.com/AccumulateNetwork/accumulate/internal/relay"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/libs/log"
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
	LogLevel         string
}

func init() {
	cmdMain.AddCommand(cmdLoadTest)

	cmdLoadTest.Flags().StringSliceVarP(&flagLoadTest.Networks, "network", "n", nil, "Network to load test (name or number)")
	cmdLoadTest.Flags().StringSliceVarP(&flagLoadTest.Remotes, "remote", "r", nil, "Node to load test, e.g. tcp://1.2.3.4:5678")
	cmdLoadTest.Flags().IntVar(&flagLoadTest.WalletCount, "wallets", 100, "Number of generated recipient wallets")
	cmdLoadTest.Flags().IntVar(&flagLoadTest.TransactionCount, "transactions", 1000, "Number of generated transactions")
	cmdLoadTest.Flags().StringVar(&flagLoadTest.LogLevel, "log-level", "disabled", "Log level")
	// cmdLoadTest.Flags().IntVar(&flagLoadTest.BatchSize, "batches", 0, "Transaction batch size; defaults to 1/5 of the transaction count")
	// cmdLoadTest.Flags().DurationVarP(&flagLoadTest.BatchDelay, "batch-delay", "d", time.Second/5, "Delay after each batch")
}

func loadTest(cmd *cobra.Command, args []string) {
	var clients []relay.Client

	if flagLoadTest.BatchSize < 1 {
		if flagLoadTest.TransactionCount > 5 {
			flagLoadTest.BatchSize = flagLoadTest.TransactionCount / 5
		} else {
			flagLoadTest.BatchSize = 1
		}
	}

	var logger log.Logger
	var err error
	if flagLoadTest.LogLevel != "disabled" {
		logger, err = log.NewDefaultLogger("plain", "info", false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to create logger: %v\n", err)
			os.Exit(1)
		}
	}

	// Create clients for networks
	for _, name := range flagLoadTest.Networks {
		net, err := networks.Resolve(flagInit.Net)
		checkf(err, "--network")

		lAddr := fmt.Sprintf("tcp://%s:%d", net.Nodes[0].IP, net.Port+networks.TmRpcPortOffset)
		client, err := rpc.New(lAddr)
		checkf(err, "failed to create RPC client for network %q", name)

		if logger != nil {
			client.SetLogger(logger)
		}
		clients = append(clients, client)
	}

	// Create clients for remotes
	for _, r := range flagLoadTest.Remotes {
		u, err := url.Parse(r)
		checkf(err, "not a valid URL: %s", r)

		if u.Path != "" && u.Path != "/" {
			fatalf("remote URL must not contain a path: %s", r)
		}

		client, err := rpc.New(r)
		checkf(err, "failed to create RPC client for remote %s", r)

		if logger != nil {
			client.SetLogger(logger)
		}
		clients = append(clients, client)
	}

	if len(clients) == 0 {
		fmt.Fprintf(os.Stderr, "Error: at least one --network or --remote is required\n")
		printUsageAndExit1(cmd, args)
	}

	relay := relay.New(connMgr, connRouter)
	query := api.NewQuery(relay)

	err = relay.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer relay.Stop()

	addrList, err := acctesting.RunLoadTest(query, flagLoadTest.WalletCount, flagLoadTest.TransactionCount)
	check(err)

	// Wait for synthetic transactions to go through
	time.Sleep(5 * time.Second)

	for _, v := range addrList[1:] {
		resp, err := query.GetChainStateByUrl(v)
		check(err)
		output, err := json.Marshal(resp)
		check(err)
		fmt.Printf("%s : %s\n", v, string(output))
	}
}
