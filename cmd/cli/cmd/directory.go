package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	url2 "github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/types"
	acmeapi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/spf13/cobra"
	"log"
)

// faucetCmd represents the faucet command
var directoryCmd = &cobra.Command{
	Use:   "directory",
	Short: "Send credits to a recipient",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			GetDirectory(args[0])
		} else {
			fmt.Println("Usage:")
			PrintDirectory()
		}
	},
}

func init() {
	rootCmd.AddCommand(directoryCmd)
}

func PrintDirectory() {
	fmt.Println("  accumulate credits [url] 		Get directory of sub-chains associate with a URL")
}

func GetDirectory(actor string) {

	u, err := url2.Parse(actor)
	if err != nil {
		PrintCredits()
		log.Fatal(err)
	}

	var res interface{}
	var str []byte

	params := acmeapi.APIRequestURL{}

	params.URL = types.String(u.String())

	if err := Client.Request(context.Background(), "get-directory", params, &res); err != nil {
		log.Fatal(err)
	}

	str, err = json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))
}
