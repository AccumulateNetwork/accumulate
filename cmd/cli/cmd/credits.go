package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	url2 "github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"log"
	"strconv"

	"github.com/spf13/cobra"
)

// faucetCmd represents the faucet command
var creditsCmd = &cobra.Command{
	Use:   "credits",
	Short: "Send credits to a recipient",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 2 {
			AddCredits(args[0], args[1], args[2])
		} else {
			fmt.Println("Usage:")
			PrintCredits()
		}
	},
}

func init() {
	rootCmd.AddCommand(creditsCmd)
}

func PrintCredits() {
	fmt.Println("  accumulate credits [fromUrl] [toUrl] [amount] 		Send credits to a recipient")
}

func AddCredits(fromUrl string, toUrl string, amount string) {

	u, err := url2.Parse(fromUrl)
	if err != nil {
		log.Fatal(err)
	}
	u2, err := url2.Parse(toUrl)
	if err != nil {
		log.Fatal(err)
	}

	u.String()
	u2.String()

	var res interface{}
	var str []byte

	amt, err := strconv.Atoi(amount)
	if err != nil {
		log.Fatal(err)
	}
	credits := protocol.AddCredits{}
	credits.Recipient = u2.String()
	credits.Amount = uint64(amt)

	data, err := json.Marshal(credits)
	if err != nil {
		log.Fatal(err)
	}

	dataBinary, err := credits.MarshalBinary()
	if err != nil {
		log.Fatal(err)
	}

	_, _, err = protocol.ParseAnonymousAddress(u)
	bucket := "adi"
	if err == nil {
		bucket = "anon"
	}

	params, err := prepareGenTx(data, dataBinary, u.String(), u.String(), bucket)
	if err != nil {
		log.Fatal(err)
	}

	if err := Client.Request(context.Background(), "add-credits", params, &res); err != nil {
		log.Fatal(err)
	}

	str, err = json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))

}
