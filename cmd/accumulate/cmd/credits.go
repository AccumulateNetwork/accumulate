package cmd

import (
	"fmt"
	"strconv"

	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/spf13/cobra"
)

// creditsCmd represents the faucet command
var creditsCmd = &cobra.Command{
	Use:   "credits",
	Short: "Send credits to a recipient",
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
	fmt.Println("  accumulate credits [origin lite account] [lite account or key page url] [amount] 		Send credits using a lite account or adi key page to another lite account or adi key page")
	fmt.Println("  accumulate credits [origin url] [origin key name] [key index (optional)] [key height (optional)] [key page or lite account url] [amount] 		Send credits to another lite account or adi key page")
	// SUGGEST: This usage does not indicate that an origin key itself can be provided in place of an origin key name.
}

// AddCredits begins execution of a transaction sending credits from one
// specified account to another. This method is called when a user enters
// the appropriate command from their CLI as defined in PrintCredits().
//
// The first parameter is the first user-supplied argument from the CLI
// and all subsequent arguments are arrayed in the second parameter.
func AddCredits(origin string, args []string) (string, error) {

	// Resolve the first user-supplied argument to a lite account or
	// ADI key page, which is indicated by a URL.
	origin_url, err := url2.Parse(origin)
	if err != nil {
		// The first argument could not be resolved to a usable URL.
		PrintCredits()
		return "", err
	}

	// Resolve the remaining user-supplied arguments to a valid target
	// lite account or ADI key page.
	args, si, privKey, err := prepareSigner(origin_url, args)
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

	amt, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return "", fmt.Errorf("amount must be an integer %v", err)
	}

	credits := protocol.AddCredits{}
	credits.Recipient = u2.String()
	credits.Amount = uint64(amt * protocol.CreditPrecision)

	res, err := dispatchTxRequest("add-credits", &credits, origin_url, si, privKey)
	if err != nil {
		return "", err
	}
	return ActionResponseFrom(res).Print()
}
