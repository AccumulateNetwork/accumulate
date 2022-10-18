// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package cmd

import (
	"fmt"
	"math/big"
	"strconv"

	"github.com/spf13/cobra"
	url2 "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// creditsCmd represents the faucet command
var creditsCmd = &cobra.Command{
	Use:   "credits [origin token account] [key page or lite identity url] [number of credits wanted] [max acme to spend]",
	Short: "Purchase credits with acme and send to recipient.",
	Args:  cobra.MinimumNArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if len(args) > 2 {
			out, err = AddCredits(args[0], args[1:])
		} else {
			fmt.Println("Usage:")
			PrintCredits()
		}
		printOutput(cmd, out, err)
	},
}

func PrintCredits() {
	fmt.Println("  accumulate credits [origin lite token account] [lite identity url or key page url] [credits desired] [max amount in acme (optional)] 		Purchase credits using a lite token account or adi key page to another lite token account or adi key page")
	fmt.Println("  accumulate credits [adi token account] [key name[@key book or page]] [key page or lite identity url] [credits desired] [max amount in acme (optional)]		Purchase credits to send to another lite identity or adi key page")
	fmt.Println("\tnote: If the max amount in ACME parameter is provided and the oracle price falls below what\n" +
		"\tthat value can cover, the transaction will fail. The minimum of the computed credit purchase and the maximum\n" +
		"\tvalue to spend will be used to satisfy the purchase.")
}

func AddCredits(origin string, args []string) (string, error) {
	u, err := url2.Parse(origin)
	if err != nil {
		PrintCredits()
		return "", err
	}

	args, signer, err := prepareSigner(u, args)
	if err != nil {
		return "", err
	}

	if len(args) < 2 {
		return "", err
	}

	u2, err := url2.Parse(args[0])
	if err != nil {
		return "", err
	}

	acmeOracle, err := QueryAcmeOracle()
	if err != nil {
		return "", err
	}

	// credits desired
	cred, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return "", err
	}

	// ACME = credits ÷ oracle ÷ credits-per-dollar
	estAcmeRat := big.NewRat(int64(cred*protocol.CreditPrecision), protocol.CreditPrecision)
	estAcmeRat.Quo(estAcmeRat, big.NewRat(int64(acmeOracle.Price), protocol.AcmeOraclePrecision))
	estAcmeRat.Quo(estAcmeRat, big.NewRat(protocol.CreditsPerDollar, 1))

	// Convert rational to an ACME balance
	estAcmeRat.Mul(estAcmeRat, big.NewRat(protocol.AcmePrecision, 1))
	acmeSpend := estAcmeRat.Num()
	if !estAcmeRat.IsInt() {
		acmeSpend.Div(acmeSpend, estAcmeRat.Denom())
	}

	//now test the cost of the credits against the max amount the user wants to spend
	if len(args) > 2 {
		maxSpend, err := amountToBigInt(protocol.ACME, args[2]) // amount in acme
		if err != nil {
			return "", fmt.Errorf("amount must be an integer %v", err)
		}

		// Fail if the estimated amount is greater than the desired max
		if acmeSpend.Cmp(maxSpend) > 0 {
			return "", fmt.Errorf("amount of credits requested will not be satisfied by amount of acme to be spent")
		}
	}

	credits := protocol.AddCredits{}
	credits.Recipient = u2
	credits.Amount = *acmeSpend
	credits.Oracle = acmeOracle.Price

	return dispatchTxAndPrintResponse(&credits, u, signer)
}
