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
	Use:   "credits [origin token account] [key page or lite account URL] [number of credits wanted] [max acme to spend] [percent slippage (optional)]",
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
	fmt.Println("  accumulate credits [origin lite token account] [lite token account or key page url] [credits desired] [max amount in acme (optional)] [percent slippage (optional)] 		Purchase credits using a lite token account or adi key page to another lite token account or adi key page")
	fmt.Println("  accumulate credits [origin url] [origin key name] [key index (optional)] [key height (optional)] [key page or lite account url] [credits desired] [max amount in acme (optional)] [percent slippage (optional)] 		Purchase credits to send to another lite token account or adi key page")
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

	var acmeOracle, _ = GetacmeOracle()

	// credits desired
	cred, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return "", err
	}

	// precision of 1 acme (Token Units / ACME)
	estAcme := big.NewInt(protocol.AcmePrecision) // Do everything with ACME precision

	// credits wanted Credit Units / dollar
	estAcme.Mul(estAcme, big.NewInt(int64(cred)))               // Credits
	estAcme.Div(estAcme, big.NewInt(protocol.CreditsPerDollar)) // Credit / Dollar

	//dollars / ACME
	estAcme.Mul(estAcme, big.NewInt(protocol.AcmeOraclePrecision)) // Oracle Precision
	estAcme.Div(estAcme, big.NewInt(int64(acmeOracle)))            // Oracle Precision * Dollars / Acme

	// token units / -ACME- * -Credits- / -Credit- * -Dollar- * -Oracle Precision- / -Dollar- * -ACME- / -Oracle Precision-

	//now test the cost of the credits against the max amount to spend
	if len(args) > 2 {
		tstAmt, err := amountToBigInt(protocol.ACME, args[2]) // amount in acme
		if err != nil {
			return "", fmt.Errorf("amount must be an integer %v", err)
		}

		//determine slippage value if applicable
		slipAmount := big.NewFloat(1.0).SetInt(tstAmt)
		if len(args) > 3 {
			slip, err := strconv.ParseFloat(args[3], 64)
			if err != nil || slip < 0.0 {
				return "", fmt.Errorf("slippage should be a percentage >= 0.0")
			}
			slip /= 100.0
			slip += 1.0
			slipAmount.Mul(slipAmount, big.NewFloat(slip))
		}

		if estAcme.Cmp(tstAmt) > 0 {
			slipAmount.Int(tstAmt)
			if estAcme.Cmp(tstAmt) > 0 {
				return "", fmt.Errorf("amount of credits requested will not be satisfied by amount of acme to be spent")
			}
		}
	}

	credits := protocol.AddCredits{}
	credits.Recipient = u2
	credits.Amount = *estAcme
	credits.Oracle = acmeOracle

	res, err := dispatchTxRequest("add-credits", &credits, nil, u, signer)
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
