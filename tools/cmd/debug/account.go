package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/client/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Analyze accounts",
}

var accountIdCmd = &cobra.Command{
	Use:   "id <url>",
	Short: "Show an account's ID",
	Args:  cobra.ExactArgs(1),
	Run:   accountId,
}

var accountRouteCmd = &cobra.Command{
	Use:   "route <network-endpoint> <url>",
	Short: "Calculate the route for an account",
	Args:  cobra.ExactArgs(2),
	Run:   accountId,
}

func init() {
	cmd.AddCommand(accountCmd)
	accountCmd.AddCommand(
		accountIdCmd,
		accountRouteCmd,
	)
}

func accountId(_ *cobra.Command, args []string) {
	var dclient *client.Client
	if len(args) == 2 {
		var err error
		dclient, err = client.New(args[0])
		check(err)
		args = args[1:]
	}

	u, err := url.Parse(args[0])
	check(err)
	fmt.Printf("Account:        %v\n", u)
	fmt.Printf("Account ID:     %X\n", u.AccountID())
	fmt.Printf("Identity ID:    %X\n", u.IdentityAccountID())
	fmt.Printf("Routing number: %X\n", u.Routing())

	// Do not route
	if dclient == nil {
		return
	}

	// Route using a routing table
	req := new(api.GeneralQuery)
	req.Url = protocol.DnUrl().JoinPath(protocol.Routing)
	resp := new(api.ChainQueryResponse)
	account := new(protocol.DataAccount)
	resp.Data = account
	err = dclient.RequestAPIv2(context.Background(), "query", req, resp)
	if err == nil {
		table := new(protocol.RoutingTable)
		check(table.UnmarshalBinary(account.Entry.GetData()[0]))
		router, err := routing.NewStaticRouter(table, nil)
		check(err)

		partition, err := router.RouteAccount(u)
		check(err)
		fmt.Printf("Method:         prefix routing table\n")
		fmt.Printf("Routes to:      %s\n", partition)
		return
	}

	var jerr *jsonrpc2.Error
	if errors.As(err, &jerr) && jerr.Code == api.ErrCodeNotFound {
		check(err)
	}

	// Route using old-style modulo routing
	info, err := dclient.Describe(context.Background())
	check(err)
	bvns := info.Network.GetBvnNames()
	nr := u.Routing() % uint64(len(bvns))
	fmt.Printf("Method:         modulo routing\n")
	fmt.Printf("Routes to:      %s\n", bvns[nr])
}
