package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
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
	var err error
	var info *api.DescriptionResponse
	bvnCount := 0
	if len(args) == 2 {
		dclient, err = client.New(args[0])
		check(err)
		info, err = dclient.Describe(context.Background())
		check(err)
		bvnCount = len(info.Describe.Subnets)
		args = args[1:]
	}

	u, err := url.Parse(args[0])
	check(err)
	fmt.Printf("Account       : %v\n", u)
	fmt.Printf("Account ID    : %X\n", u.AccountID())
	fmt.Printf("Identity ID   : %X\n", u.IdentityAccountID())
	fmt.Printf("Routing number: %X\n", u.Routing())
	if bvnCount != 0 {
		subnet, err := routing.RouteAccount(&info.Describe.Network, u)
		check(err)
		fmt.Printf("Routes to     : %s\n", subnet)
	}
}
