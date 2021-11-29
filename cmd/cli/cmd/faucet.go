package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/types"
	acmeapi "github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/spf13/cobra"
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

	var res acmeapi.APIDataResponse
	params := acmeapi.APIRequestURL{}
	u, err := url2.Parse(url)
	if err != nil {
		return "", err
	}
	params.URL = types.String(u.String())

	if err := Client.Request(context.Background(), "faucet", params, &res); err != nil {
		return PrintJsonRpcError(err)
	}
	ar := ActionResponse{}
	err = json.Unmarshal(*res.Data, &ar)
	if err != nil {
		return "", fmt.Errorf("error unmarshalling create adi result")
	}
	return ar.Print()
}
