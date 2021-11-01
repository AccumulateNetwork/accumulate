package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	url2 "github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/spf13/cobra"
)

// faucetCmd represents the faucet command
var creditsCmd = &cobra.Command{
	Use:   "credits",
	Short: "Send credits to a recipient",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 2 {
			AddCredits(args[0], args[1:])
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
	fmt.Println("  accumulate credits [actor lite account] [lite account or key page url] [amount] 		Send credits using a lite account or adi key page to another lite account or adi key page")
	fmt.Println("  accumulate credits [actor url] [actor key label] [key index (optional)] [key height (optional)] [key page or lite account url] [amount] 		Send credits to a recipient")
}

func AddCredits(actor string, args []string) {

	u, err := url2.Parse(actor)
	if err != nil {
		PrintCredits()
		log.Fatal(err)
	}

	args, si, privKey, err := prepareSigner(u, args)
	if err != nil {
		PrintCredits()
		log.Fatal(err)
	}

	if len(args) < 2 {
		PrintCredits()
		log.Fatal(err)
	}

	u2, err := url2.Parse(args[0])
	if err != nil {
		PrintCredits()
		log.Fatal(err)
	}

	amt, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		PrintCredits()
		log.Fatal(fmt.Errorf("amount must be an integer %v", err))
	}
	var res interface{}
	var str []byte

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

	nonce := uint64(time.Now().Unix())
	params, err := prepareGenTx(data, dataBinary, u, si, privKey, nonce)
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
