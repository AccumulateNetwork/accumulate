package cmd

import (
	"context"
	"encoding/json"
	"fmt"

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
		if err != nil {
			cmd.Print("Error: ")
			cmd.PrintErr(err)
		} else {
			cmd.Println(out)
		}
	},
}

func PrintFaucet() {
	fmt.Println("  accumulate faucet [url] 		Get tokens from faucet to address")
}

func Faucet(url string) (string, error) {

	var res acmeapi.APIDataResponse
	params := acmeapi.APIRequestURL{}
	params.URL = types.String(url)

	if err := Client.Request(context.Background(), "faucet", params, &res); err != nil {
		return PrintJsonRpcError(err)
	}
	ar := ActionResponse{}
	err := json.Unmarshal(*res.Data, &ar)
	if err != nil {
		return "", fmt.Errorf("error unmarshalling create adi result")
	}
	return ar.Print()
}
