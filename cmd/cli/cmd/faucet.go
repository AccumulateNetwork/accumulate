package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/AccumulateNetwork/accumulated/types"
	acmeapi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/spf13/cobra"
)

// faucetCmd represents the faucet command
var faucetCmd = &cobra.Command{
	Use:   "faucet",
	Short: "Get tokens from faucet",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			Faucet(args[0])
		} else {
			fmt.Println("Usage:")
			PrintFaucet()
		}
	},
}

func init() {
	rootCmd.AddCommand(faucetCmd)
}

func PrintFaucet() {
	fmt.Println("  accumulate faucet [url] 		Get tokens from faucet to address")
}

func Faucet(url string) {

	var res acmeapi.APIDataResponse
	params := acmeapi.APIRequestURL{}
	params.URL = types.String(url)

	if err := Client.Request(context.Background(), "faucet", params, &res); err != nil {
		log.Fatal(err)
	}
	ar := ActionResponse{}
	err := json.Unmarshal(*res.Data, &ar)
	if err != nil {
		log.Fatal("error unmarshalling create adi result")
	}
	ar.Print()
}
