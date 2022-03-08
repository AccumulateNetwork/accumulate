package cmd

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/spf13/cobra"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// creditsCmd represents the faucet command
var creditsCmd = &cobra.Command{
	Use:   "credits",
	Short: "Purchase credits with acme and send to recipient.",
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
	fmt.Println("  accumulate credits [origin lite token account] [lite token account or key page url] [amount] 		Send credits using a lite token account or adi key page to another lite token account or adi key page")
	fmt.Println("  accumulate credits [origin url] [origin key name] [key index (optional)] [key height (optional)] [key page or lite account url] [amount] 		acme to credits to another lite token account or adi key page")
}

func AddCredits(origin string, args []string) (string, error) {
	u, err := url2.Parse(origin)
	if err != nil {
		PrintCredits()
		return "", err
	}

	args, si, privKey, err := prepareSigner(u, args)
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

	//amt, err := strconv.ParseFloat(args[1], 64)
	amt, err := amountToBigInt(protocol.ACME, args[1])
	if err != nil {
		return "", fmt.Errorf("amount must be an integer %v", err)
	}

	credits := protocol.AddCredits{}
	credits.Recipient = u2
	credits.Amount = *new(big.Int).SetUint64(uint64(amt * protocol.CreditPrecision))

	res, err := dispatchTxRequest("add-credits", &credits, nil, u, si, privKey)
	if err != nil {
		return "", err
	}
	if !TxNoWait && TxWait > 0 {
		_, err := waitForTxn(res.TransactionHash, TxWait)
		if err != nil {
			var rpcErr jsonrpc2.Error
			if errors.As(err, &rpcErr) {
				return PrintJsonRpcError(err)
			}
			return "", err
		}
	}
	return ActionResponseFrom(res).Print()
}
