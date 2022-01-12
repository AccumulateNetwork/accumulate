package cmd

import (
	"fmt"
	flag "github.com/spf13/pflag"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/spf13/cobra"
)

var stakingCmd = &cobra.Command{
	Use:   "staking",
	Short: "Staking and Validator commands",
	DisableFlagParsing:         true,
	SuggestionsMinimumDistance: 2,
	Run: func(cmd *cobra.Command, args []string) {
		
	
		if len(args) == 2 {
		CreateVal(cmd.Flags())
		}
		cmd.Flags().AddFlagSet(FlagSetPublicKey())
		cmd.Flags().AddFlagSet(FlagSetAmount())
		cmd.Flags().AddFlagSet(flagSetDescriptionCreate())
		cmd.Flags().AddFlagSet(FlagSetCommissionCreate())
//		NewCreateValidatorCmd(),
	
		_ = cmd.MarkFlagRequired(FlagAmount)
		_ = cmd.MarkFlagRequired(FlagPubKey)
		_ = cmd.MarkFlagRequired(FlagMoniker)
	},

	
	
}


const (
	FlagAddressValidator    = "validator"
	FlagAddressValidatorSrc = "addr-validator-source"
	FlagAddressValidatorDst = "addr-validator-dest"
	FlagPubKey              = "pubkey"
	FlagAmount              = "amount"
	FlagSharesAmount        = "shares-amount"
	FlagSharesFraction      = "shares-fraction"

	FlagMoniker         = "moniker"
	FlagEditMoniker     = "new-moniker"
	FlagIdentity        = "identity"
	FlagWebsite         = "website"
	FlagSecurityContact = "security-contact"
	FlagDetails         = "details"

	FlagCommissionRate          = "commission-rate"
	FlagCommissionMaxRate       = "commission-max-rate"
	FlagCommissionMaxChangeRate = "commission-max-change-rate"

	FlagMinSelfDelegation = "min-self-delegation"

	FlagGenesisFormat = "genesis-format"
	FlagNodeID        = "node-id"
	FlagIP            = "ip"
)

func flagSetDescriptionCreate() *flag.FlagSet {
	fs := flag.NewFlagSet("", flag.ContinueOnError)

	fs.String(FlagMoniker, "", "The validator's name")
	fs.String(FlagIdentity, "", "The optional identity signature (ex. UPort or Keybase)")
	fs.String(FlagWebsite, "", "The validator's (optional) website")
	fs.String(FlagSecurityContact, "", "The validator's (optional) security contact email")
	fs.String(FlagDetails, "", "The validator's (optional) details")

	return fs
}

// FlagSetCommissionCreate Returns the FlagSet used for commission create.
func FlagSetCommissionCreate() *flag.FlagSet {
	fs := flag.NewFlagSet("", flag.ContinueOnError)

	fs.String(FlagCommissionRate, "", "The initial commission rate percentage")
	fs.String(FlagCommissionMaxRate, "", "The maximum commission rate percentage")
	fs.String(FlagCommissionMaxChangeRate, "", "The maximum commission change rate percentage (per day)")

	return fs
}

// FlagSetPublicKey Returns the flagset for Public Key related operations.
func FlagSetPublicKey() *flag.FlagSet {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.String(FlagPubKey, "", "The validator's Protobuf JSON encoded public key")
	return fs
}

// FlagSetAmount Returns the FlagSet for amount related operations.
func FlagSetAmount() *flag.FlagSet {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.String(FlagAmount, "", "Amount of coins to bond")
	return fs
}




// CreateKeyBook create a new key page
func CreateVal(fs *flag.FlagSet) (*protocol.CreateValidator, error) {
	cv := protocol.CreateValidator{}
	fAmount, _ := fs.GetString(FlagAmount)

	
	
	//vAddr get from addy
	fPubKey, _ := fs.GetString(FlagPubKey)
	fmt.Println( fAmount, fPubKey)

	var pubKey []byte

	moniker, _ := fs.GetString(FlagMoniker)
	identity, _ := fs.GetString(FlagIdentity)
	website, _ := fs.GetString(FlagWebsite)
	details, _ := fs.GetString(FlagDetails)
	description := protocol.NewDescription(
		moniker,
		identity,
		website,
		details,	
	)
	vv := new(protocol.ValidatorType)
	val := vv.OperatorAddress
		// get the validator commission
	//	rate, _ := fs.GetString(FlagCommissionRate)
	//	maxRate, _ := fs.GetString(FlagCommissionMaxRate)
	//	maxChangeRate, _ := fs.GetString(FlagCommissionMaxChangeRate)
	commish := vv.Commission

	create, err := cv.NewCreateValidator( pubKey, description, commish, val, cv.Amount)
	if err != nil {
		return nil, err
	}
	return create, nil
}

// Return the flagset, particular flags, and a description of defaults
// this is anticipated to be used with the gen-tx
func CreateValidatorMsgFlagSet(ipDefault string) (fs *flag.FlagSet, defaultsDesc string) {
	fsCreateValidator := flag.NewFlagSet("", flag.ContinueOnError)
	fsCreateValidator.String(FlagIP, ipDefault, "The node's public IP")
	fsCreateValidator.String(FlagNodeID, "", "The node's NodeID")
	fsCreateValidator.String(FlagMoniker, "", "The validator's (optional) moniker")
	fsCreateValidator.String(FlagWebsite, "", "The validator's (optional) website")
	fsCreateValidator.String(FlagSecurityContact, "", "The validator's (optional) security contact email")
	fsCreateValidator.String(FlagDetails, "", "The validator's (optional) details")
	fsCreateValidator.String(FlagIdentity, "", "The (optional) identity signature (ex. UPort or Keybase)")
	fsCreateValidator.AddFlagSet(FlagSetCommissionCreate())
	fsCreateValidator.AddFlagSet(FlagSetAmount())
	fsCreateValidator.AddFlagSet(FlagSetPublicKey())

	defaultsDesc = fmt.Sprintf(`
	delegation amount:           %s
	commission rate:             %s
	commission max rate:         %s
	commission max change rate:  %s
	minimum self delegation:     %s
`, defaultAmount, defaultCommissionRate,
		defaultCommissionMaxRate, defaultCommissionMaxChangeRate,
		defaultMinSelfDelegation)

	return fsCreateValidator, defaultsDesc
}
// default values
var (
	DefaultTokens                  = 10
	defaultAmount                  = 1
	defaultCommissionRate          = "0.1"
	defaultCommissionMaxRate       = "0.2"
	defaultCommissionMaxChangeRate = "0.01"
	defaultMinSelfDelegation       = "1"
)