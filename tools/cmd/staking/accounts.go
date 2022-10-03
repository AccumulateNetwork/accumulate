package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/staking"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/staking/network"
)

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Add, remove, or list approved stakers",
}

var accountListCmd = &cobra.Command{
	Use:   "list",
	Short: "List approved stakers",
	Run:   listAccounts,
}

var accountAddCmd = &cobra.Command{Use: "add", Short: "Add a staker"}
var accountAddCoreCmd = &cobra.Command{Use: "core", Short: "Add a core staker"}
var accountAddStakingCmd = &cobra.Command{Use: "staking", Short: "Add a staking staker"}

var accountAddCoreValidatorCmd = &cobra.Command{
	Use:   "validator [identity] [stake] [rewards]",
	Short: "Add a core validator staker",
	Run:   add(staking.AccountTypeCoreValidator),
}

var accountAddCoreFollowerCmd = &cobra.Command{
	Use:   "follower [identity] [stake] [rewards]",
	Short: "Add a core follower staker",
	Run:   add(staking.AccountTypeCoreFollower),
}

var accountAddStakingValidatorCmd = &cobra.Command{
	Use:   "validator [identity] [stake] [rewards]",
	Short: "Add a staking validator staker",
	Run:   add(staking.AccountTypeStakingValidator),
}

var accountAddPureCmd = &cobra.Command{
	Use:   "pure [identity] [stake] [rewards]",
	Short: "Add a pure staker",
	Run:   add(staking.AccountTypePure),
}

var accountAddDelegatedCmd = &cobra.Command{
	Use:   "delegated [identity] [stake] [rewards] [delegate]",
	Short: "Add a delegated staker",
	Run:   add(staking.AccountTypeDelegated),
}

func init() {
	cmd.AddCommand(accountCmd)
	accountCmd.AddCommand(accountListCmd, accountAddCmd)
	accountAddCmd.AddCommand(accountAddCoreCmd, accountAddStakingCmd, accountAddPureCmd, accountAddDelegatedCmd)
	accountAddCoreCmd.AddCommand(accountAddCoreValidatorCmd, accountAddCoreFollowerCmd)
	accountAddStakingCmd.AddCommand(accountAddStakingValidatorCmd)
}

func listAccounts(*cobra.Command, []string) {
	net := network.New(theClient, flag.Parameters)
	params, err := net.GetParameters()
	checkf(err, "get parameters")

	req := new(api.DataEntrySetQuery)
	req.Url = params.Account.ApprovedADIs
	req.Expand = true
	req.Count = 10
	for {
		res, err := theClient.QueryDataSet(context.Background(), req)
		checkf(err, "query approved")

		for _, item := range res.Items {
			b, err := json.Marshal(item)
			check(err)
			entry := new(client.DataEntryQueryResponse)
			check(json.Unmarshal(b, entry))

			b = entry.Entry.GetData()[0]
			account := new(staking.Account)
			check(account.UnmarshalBinary(b))

			fmt.Printf("%v %v\n", account.Type, account.Identity)
		}

		res.Start += uint64(len(res.Items))
		if res.Start >= res.Total {
			break
		}
	}
}

func add(typ staking.AccountType) func(_ *cobra.Command, args []string) {
	return func(_ *cobra.Command, args []string) {
		account := new(staking.Account)
		account.Type = typ

		var err error
		account.Identity, err = url.Parse(args[0])
		checkf(err, "identity")

		account.Stake, err = url.Parse(args[1])
		checkf(err, "stake")

		account.Rewards, err = url.Parse(args[2])
		checkf(err, "rewards")

		if typ == staking.AccountTypeDelegated {
			account.Delegate, err = url.Parse(args[3])
			checkf(err, "delegate")
		}

		b, err := account.MarshalBinary()
		checkf(err, "marshal account")

		entry := new(protocol.AccumulateDataEntry)
		entry.Data = [][]byte{b}
		wd := new(protocol.WriteData)
		wd.Entry = entry

		b, err = json.Marshal(wd)
		checkf(err, "marshal transaction body")

		net := network.New(theClient, flag.Parameters)
		params, err := net.GetParameters()
		checkf(err, "get parameters")
		fmt.Printf("accumulate tx execute %v YourKey '%s'\n", params.Account.ApprovedADIs, b)
	}
}
