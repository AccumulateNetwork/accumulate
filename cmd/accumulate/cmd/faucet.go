package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	api2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
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
	var res api2.TxResponse
	params := api2.UrlQuery{}

	u, err := url2.Parse(url)
	if err != nil {
		return "", err
	}

	params.Url = u

	data, err := json.Marshal(&params)
	if err != nil {
		return "", err
	}
	if err := Client.RequestAPIv2(context.Background(), "faucet", json.RawMessage(data), &res); err != nil {
		return PrintJsonRpcError(err)
	}

	return ActionResponseFrom(&res).Print()

}
