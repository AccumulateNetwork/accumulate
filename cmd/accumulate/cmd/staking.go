package cmd

import (
	"encoding/json"
	"fmt"

	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/spf13/cobra"
)

// WORK IN PROGRESS

var stakingCmd = &cobra.Command{
	Use:   "staking",
	Short: "Staking and Validator commands",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "create-validator":
				if len(args) > 2 {
						out, err = CreateVal(args[1], args[2:])
					} else {
						fmt.Println("Usage:")
						PrintCreateValidator() 
					}
			default:
						fmt.Printf("\nTo create a Validator, you need to pass in moniker, publicKey, amountStaking, commission, and validatorAddress\n\n")
					} 
		} else {
			fmt.Println("Usage")
			PrintValidators()

				}
			//PrintCreateValidator()
		 
			printOutput(cmd, out, err)
		},

	}
		
	

func PrintCreateValidator() {
	fmt.Println("  accumulate staking create-validator [moniker] [identity] [website] [details] [commission] [validator-address] [amount] [pubkey]")
	fmt.Println("\t\t example usage: accumulate staking create-validator myValName...")
}

func PrintValidators() {
	PrintCreateValidator()
}


// CreateKeyBook create a new key page
func CreateVal(sender string, args []string) (string, error) {
	/*
	cv := new(protocol.CreateValidator)



	var pubKey []byte
	moniker := cv.Description.Moniker
	identity := cv.Description.Identity
	website := cv.Description.Website
	details := cv.Description.Details

	description := protocol.NewDescription(
		moniker,
		identity,
		website,
		details,	
	)

	moniker = args[0]
	if err != nil {
		PrintAccountCreate()
		return "", fmt.Errorf("invalid account url %s", args[0])
	}
	val := cv.ValidatorAddress
		// get the validator commission
	//	rate, _ := fs.GetString(FlagCommissionRate)
	//	maxRate, _ := fs.GetString(FlagCommissionMaxRate)
	//	maxChangeRate, _ := fs.GetString(FlagCommissionMaxChangeRate)
	commish := cv.Commission

	create, err := cv.NewCreateValidator( pubKey, description, commish, val, cv.Amount)
	if err != nil {
		return "", err
	}
*/
	u, err := url2.Parse(sender)
	if err != nil {
		return "", err
	}

	args, si, pk, err := prepareSigner(u, args)
	if err != nil {
		return "", fmt.Errorf("unable to prepare signer, %v", err)
	}


	var ty struct {
		Type types.TransactionType
	}

	err = json.Unmarshal([]byte(args[0]), &ty)
	if err != nil {
		return "", fmt.Errorf("invalid payload 1: %v", err)
	}

	txn, err := protocol.NewTransaction(ty.Type)
	if err != nil {
		return "", fmt.Errorf("invalid payload 2: %v", err)
	}

	err = json.Unmarshal([]byte(args[0]), txn)
	if err != nil {
		return "", fmt.Errorf("invalid payload 3: %v", err)
	}

	res, err := dispatchTxRequest("execute", txn, u, si, pk)
	if err != nil {
		return "", err
	}
	return ActionResponseFrom(res).Print()
}
