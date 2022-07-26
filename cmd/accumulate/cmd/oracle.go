package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var oracleCmd = &cobra.Command{
	Use:   "oracle",
	Short: "Send credits to a recipient",
	Args:  cobra.ExactArgs(0),
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
	acmeOracle, err := QueryAcmeOracle()
	if err != nil {
		return "", err
	}

	if WantJsonOutput {
		data, err := json.Marshal(&acmeOracle)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	usd := float64(acmeOracle.Price) / protocol.AcmeOraclePrecision
	credits := (usd * protocol.CreditUnitsPerFiatUnit) / protocol.CreditPrecision
	out := "USD per ACME : $" + strconv.FormatFloat(usd, 'f', 4, 64)
	out += "\nCredits per ACME : " + strconv.FormatFloat(credits, 'f', 2, 64)

	return out, nil
}
