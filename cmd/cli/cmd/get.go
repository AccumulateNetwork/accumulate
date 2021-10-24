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

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Get data by URL",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			Get(args[0])
		} else {
			fmt.Println("Usage:")
			PrintGet()
		}
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
}

func PrintGet() {
	fmt.Println("  accumulate get [url] 		Get data by Accumulate URL")
}

func Get(url string) {

	var res interface{}
	var str []byte

	params := acmeapi.APIRequestURL{}
	params.URL = types.String(url)

	str1, err1 := json.Marshal(&params)
	if err1 != nil {
		log.Fatal(err1)
	}

	fmt.Println(string(str1))

	if err := Client.Request(context.Background(), "get", params, &res); err != nil {
		log.Fatal(err)
	}

	str, err := json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))

}
