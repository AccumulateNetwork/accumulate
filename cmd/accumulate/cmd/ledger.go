package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd"
	accounts "gitlab.com/accumulatenetwork/ledger/ledger-go-accumulate"
)

var walletID string

var ledgerCmd = &cobra.Command{
	Use:   "ledger",
	Short: "ledger command",
	Args:  cobra.MinimumNArgs(3),
}

var ledgerInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "get list of wallets with their info",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := queryWalletsInfo(cmd, args)
		printOutput(cmd, out, err)
	},
}

var ledgerKeyGenerateCmd = &cobra.Command{
	Use:   "key generate [key name/label]",
	Short: "generate keypair on a ledger device and give it a name",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := legerGenerateKey(cmd, args)
		printOutput(cmd, out, err)
	},
}

func init() {
	initRunFlags(ledgerCmd, false)
	ledgerCmd.AddCommand(ledgerInfoCmd)
	ledgerCmd.AddCommand(ledgerKeyGenerateCmd)
	ledgerKeyGenerateCmd.Flags().StringVar(&walletID, "wallet-id", "", "specify the wallet id by \"ledger info\" index or ledger ID URL")
}

func queryWalletsInfo(cmd *cobra.Command, args []string) (string, error) {
	ledgerApi, err := walletd.NewLedgerApi()
	if err != nil {
		return "", err
	}
	ledgerInfos, err := ledgerApi.QueryLedgerWalletsInfo()
	if err != nil {
		return "", err
	}

	if WantJsonOutput {
		str, err := json.Marshal(ledgerInfos)
		if err != nil {
			return "", err
		}
		return string(str), nil
	} else {
		result := fmt.Sprintln("Wallets:")
		for i, ledgerInfo := range ledgerInfos {
			result += fmt.Sprintf("%d\tManufacturer:\t%s\n", i+1, ledgerInfo.Manufacturer)
			result += fmt.Sprintf("\tProduct:\t%s\n", ledgerInfo.Product)
			result += fmt.Sprintf("\tVendor ID:\t%d\n", ledgerInfo.VendorID)
			result += fmt.Sprintf("\tProduct ID:\t%d\n", ledgerInfo.ProductID)
			result += fmt.Sprintf("\tApp Version:\t%s\n", ledgerInfo.Version.Label)
			result += fmt.Sprintf("\tLedger ID:\t%s\n", ledgerInfo.Url)
		}
		return result, nil
	}
}

func legerGenerateKey(cmd *cobra.Command, args []string) (string, error) {
	ledgerApi, err := walletd.NewLedgerApi()
	if err != nil {
		return "", err
	}

	selWallet, err := selectWallet(ledgerApi)
	if err != nil {
		return "", err
	}

	label := args[1]
	keyData, err := ledgerApi.GenerateKey(selWallet, label)

	if WantJsonOutput {
		str, err := json.Marshal(keyData)
		if err != nil {
			return "", err
		}

		return string(str), nil
	}

	return fmt.Sprintf("\tname\t\t:\t%s\n\twallet ID\t:\t%s\n\tpublic key\t:\t%x\n\tkey type\t:\t%s\n", label,
		keyData.WalletID, keyData.PublicKey, keyData.KeyType), nil
}

func selectWallet(ledgerApi *walletd.LedgerApi) (accounts.Wallet, error) {
	wallets := ledgerApi.Wallets()
	walletCnt := len(wallets)
	switch {
	case walletCnt == 1:
		return wallets[0], nil
	case walletCnt == 0:
		return nil, errors.New("no wallets found, please check if your wallet and the Accumulate app on it are online")
	case walletCnt > 1 && len(walletID) == 0:
		return nil, errors.New(
			fmt.Sprintf("there is more than wallets available (%d), please use the --wallet-id flag to select the correct wallet", walletCnt))
	}

	var selWallet accounts.Wallet
	for i, wallet := range wallets {
		if strings.HasPrefix(walletID, "ledger://") {
			wid := wallet.URL()
			if wid.String() == walletID {
				selWallet = wallet
				break
			}
		} else {
			if walletIdx, err := strconv.Atoi(walletID); err == nil {
				if walletIdx == i+1 {
					selWallet = wallet
					break
				}
			}
		}
	}
	if selWallet == nil {
		return nil, errors.New(
			fmt.Sprintf("no wallet with ID %s could be found, please use accumulate ledger info to identify the connected wallets", walletID))
	}
	return selWallet, nil
}
