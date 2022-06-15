package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	authCmd.AddCommand(
		authEnableCmd,
		authDisableCmd,
		authAddCmd,
		authRemoveCmd,
	)
}

var authCmd = &cobra.Command{
	Use:     "auth",
	Short:   "Manage authorization of an account",
	Aliases: []string{"manager"},
}

var authEnableCmd = &cobra.Command{
	Use:   "enable [account url] [signing key name] [key index (optional)] [key height (optional)] [authority url or index (1-based)]",
	Short: "Enable authorization checks for an authority of an account",
	Args:  cobra.RangeArgs(3, 5),
	Run:   runTxnCmdFunc(EnableAuth),
}

var authDisableCmd = &cobra.Command{
	Use:   "disable [account url] [signing key name] [key index (optional)] [key height (optional)] [authority url or index (1-based)]",
	Short: "Disable authorization checks for an authority of an account",
	Args:  cobra.RangeArgs(3, 5),
	Run:   runTxnCmdFunc(DisableAuth),
}

var authAddCmd = &cobra.Command{
	Use:     "add [account url] [signing key name] [key index (optional)] [key height (optional)] [authority url]",
	Short:   "Add an authority to an account",
	Aliases: []string{"set"},
	Args:    cobra.RangeArgs(3, 5),
	Run:     runTxnCmdFunc(AddAuth),
}

var authRemoveCmd = &cobra.Command{
	Use:   "remove [account url] [signing key name] [key index (optional)] [key height (optional)] [authority url or index (1-based)]",
	Short: "Remove an authority from an account",
	Args:  cobra.RangeArgs(3, 5),
	Run:   runTxnCmdFunc(RemoveAuth),
}

func getAccountAuthority(accountUrl *url2.URL, name string) (*url2.URL, error) {
	account, err := getAccount(accountUrl.String())
	if err != nil {
		return nil, err
	}

	fullAccount, ok := account.(protocol.FullAccount)
	if !ok {
		return nil, fmt.Errorf("account type %v does not support advanced auth", account.Type())
	}
	auth := fullAccount.GetAuth()

	i, err := strconv.ParseUint(name, 10, 8)
	if err != nil {
		return url2.Parse(name)
	}

	if i == 0 || int(i) > len(auth.Authorities) {
		return nil, fmt.Errorf("%d is not a valid account authority number", i)
	}

	return auth.Authorities[i-1].Url, nil
}

func EnableAuth(account *url2.URL, signers []*signing.Builder, args []string) (string, error) {
	authority, err := getAccountAuthority(account, args[0])
	if err != nil {
		return "", err
	}

	op := &protocol.EnableAccountAuthOperation{Authority: authority}
	txn := &protocol.UpdateAccountAuth{Operations: []protocol.AccountAuthOperation{op}}
	return dispatchTxAndPrintResponse(txn, account, signers)
}

func DisableAuth(account *url2.URL, signers []*signing.Builder, args []string) (string, error) {
	authority, err := getAccountAuthority(account, args[0])
	if err != nil {
		return "", err
	}

	op := &protocol.DisableAccountAuthOperation{Authority: authority}
	txn := &protocol.UpdateAccountAuth{Operations: []protocol.AccountAuthOperation{op}}
	return dispatchTxAndPrintResponse(txn, account, signers)
}

func AddAuth(account *url2.URL, signers []*signing.Builder, args []string) (string, error) {
	authority, err := getAccountAuthority(account, args[0])
	if err != nil {
		return "", err
	}

	op := &protocol.AddAccountAuthorityOperation{Authority: authority}
	txn := &protocol.UpdateAccountAuth{Operations: []protocol.AccountAuthOperation{op}}
	return dispatchTxAndPrintResponse(txn, account, signers)
}

func RemoveAuth(account *url2.URL, signers []*signing.Builder, args []string) (string, error) {
	authority, err := getAccountAuthority(account, args[0])
	if err != nil {
		return "", err
	}

	op := &protocol.RemoveAccountAuthorityOperation{Authority: authority}
	txn := &protocol.UpdateAccountAuth{Operations: []protocol.AccountAuthOperation{op}}
	return dispatchTxAndPrintResponse(txn, account, signers)
}
