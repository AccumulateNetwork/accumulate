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
	originURL, err := url2.Parse(origin)
	if err != nil {
		// The first argument could not be resolved to a usable URL.
		PrintCredits()
		return "", err
	}

	// Resolve the remaining user-supplied arguments to a valid target
	// lite account or ADI key page.
	// Note that the args used to do this are removed from the arg list.
	args, trxHeader, privKey, err := prepareSigner(originURL, args)
	if err != nil {
		return "", err
	}

	// Check how many args were NOT used by prepareSigner() to resolve a
	// signing key. There must be at least two: a target and an amount.
	if len(args) < 2 {
		return "", err
	}

	// The target must be specified by a URL in a single arg.
	targetURL, err := url2.Parse(args[0])
	if err != nil {
		return "", err
	}

	// Read the amount of credits for this transaction.
	amount, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return "", fmt.Errorf("amount must be an integer %v", err)
	}

	payload := protocol.AddCredits{}
	payload.Recipient = targetURL.String()
	payload.Amount = uint64(amount * protocol.CreditPrecision)

	// Send the transaction request.
	res, err := dispatchTxRequest("add-credits", &payload, originURL, trxHeader, privKey)
	if err != nil {
		return "", err
	}

	// Return the response from transaction execution.
	// SUGGEST: Print() doesn't actually print anything to system output,
	// it returns a String. Consider renaming.
	return ActionResponseFrom(res).Print()
}
