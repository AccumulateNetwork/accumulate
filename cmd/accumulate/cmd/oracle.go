package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var oracleCmd = &cobra.Command{
	Use:   "oracle",
	Short: "Send credits to a recipient",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if len(args) == 1 {
			out, err = GetCreditValue(args[0])
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

func GetCreditValue(accountUrl string) (string, error) {
	u, err := url.Parse(accountUrl)
	if err != nil {
		return "", err
	}

	params := api.DataEntryQuery{}
	params.Url = u

	var res protocol.AcmeOracle

	data, err := json.Marshal(&params)
	if err != nil {
		return "", err
	}

	err = Client.RequestAPIv2(context.Background(), "query-data", json.RawMessage(data), &res)
	if err != nil {
		return "", err
	}

	credits := res.Price * uint64(protocol.AcmeOraclePrecision)
	out := "credits per ACME : " + strconv.Itoa(int(credits))
	return out, nil
}
