package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	url2 "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// faucetCmd represents the faucet command
var faucetCmd = &cobra.Command{
	Use:   "faucet",
	Short: "Get tokens from faucet",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if len(args) > 0 {
			out, err = Faucet(args[0])
		} else {
			fmt.Println("Usage:")
			PrintFaucet()
		}
		printOutput(cmd, out, err)
	},
}

func PrintFaucet() {
	fmt.Println("  accumulate faucet [url] 		Get tokens from faucet to address")
}

func Faucet(url string) (string, error) {
	u, err := url2.Parse(url)
	if err != nil {
		return "", err
	}

	res, err := Client.Faucet(context.Background(), &protocol.AcmeFaucet{Url: u})
	if err != nil {
		return PrintJsonRpcError(err)
	}

	if TxWait != 0 {
		_, err = waitForTxnUsingHash(res.TransactionHash, TxWait, true)
		if err != nil {
			return PrintJsonRpcError(err)
		}
	}

	return ActionResponseFrom(res).Print()
}
