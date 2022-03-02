package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var oracleCmd = &cobra.Command{
	Use:   "oracle",
	Short: "Send credits to a recipient",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if len(args) == 0 {
			out, err = GetCreditValue()
		} else {
			fmt.Println("Usage:")
			PrintOracles()
		}
		printOutput(cmd, out, err)
	},
}

func PrintOracles() {
	fmt.Println("  accumulate oracle [DataAccountURL] 		Get Credits per ACME")
}

func GetCreditValue() (string, error) {
	params := api.DataEntryQuery{}
	params.Url = protocol.PriceOracle()

	res := new(api.ChainQueryResponse)
	entry := new(api.DataEntryQueryResponse)
	res.Data = entry

	err := Client.RequestAPIv2(context.Background(), "query-data", &params, &res)
	if err != nil {
		return "", err
	}

	var acmeOracle protocol.AcmeOracle
	if err = json.Unmarshal(entry.Entry.Data, &acmeOracle); err != nil {
		return "", err
	}

	usd := float64(acmeOracle.Price) / protocol.AcmeOraclePrecision
	credits := (float64(acmeOracle.Price) * protocol.CreditsPerFiatUnit) / protocol.CreditPrecision
	out := "USD per ACME : " + strconv.FormatFloat(usd, 'f', 4, 64)
	out += "\nCredits per ACME : " + strconv.FormatFloat(credits, 'f', 2, 64)
	return out, nil
}
