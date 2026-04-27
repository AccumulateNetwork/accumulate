//go:build ignore

// One-off helper to list an account's chains. Build with:
//   go run ./cmd/backwalk-probe/list.go -account dn.acme/operators
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func main() {
	network := flag.String("network", "mainnet", "endpoint")
	acctFlag := flag.String("account", "dn.acme/operators", "account url")
	flag.Parse()

	endpoint := accumulate.ResolveWellKnownEndpoint(*network, "v3")
	client := jsonrpc.NewClient(endpoint)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	acct, err := url.Parse(*acctFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	q := api.Querier2{Querier: client}
	chains, err := q.QueryAccountChains(ctx, acct, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query account chains: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Account: %s  chains=%d\n", acct, len(chains.Records))
	for _, c := range chains.Records {
		fmt.Printf("  %-30s  type=%-20s  count=%d\n", c.Name, c.Type, c.Count)
	}
}
