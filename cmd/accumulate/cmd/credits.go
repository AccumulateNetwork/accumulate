package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// creditsCmd represents the faucet command
var creditsCmd = &cobra.Command{
	Use:   "credits [origin token account] [key page or lite identity url] [number of credits wanted] [max acme to spend] [percent slippage (optional)]",
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
	fmt.Println("  accumulate credits [origin url] [origin key name] [key index (optional)] [key height (optional)] [key page or lite identity url] [credits desired] [max amount in acme (optional)]		Purchase credits to send to another lite identity or adi key page")
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
	estAcmeRat := core.NewBigRat(int64(cred*protocol.CreditPrecision), protocol.CreditPrecision)
	estAcmeRat = estAcmeRat.Div2(int64(acmeOracle.Price), protocol.AcmeOraclePrecision)
	estAcmeRat = estAcmeRat.Div2(protocol.CreditsPerDollar, 1)

	// Convert rational to an ACME balance
	acmeSpend := estAcmeRat.Mul2(protocol.AcmePrecision, 1).Int()

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
