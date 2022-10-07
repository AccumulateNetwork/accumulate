// File holds interfaces and constants

package app

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

const ( // Account Types
	PureStaker        = "Pure Staker"
	ProtocolValidator = "Protocol Validator"
	ProtocolFollower  = "Protocol Follower"
	StakingValidator  = "Staking Validator"
	Delegate          = "Delegate"
	Data              = "Data"

	SecondsPerMajor = 5
)

type Accumulate interface {
	Run()                                                          // Start the monitor (or simulation)
	GetParameters() (*Parameters, error)                           // Get Staking App parameters from the protocol
	GetBlock(index int64, accounts map[string]int) (*Block, error) // Get the Major Block at the given index, and return account info
	GetTokensIssued() (int64, error)                               // Return the Acme Tokens Issued
	TokensIssued(int64)                                            // Report tokens issued. Simulator needs this, not the protocol
}

// Important Staking URLs.
var AcmeTokenIssuanceAccount,
	AccumulateStakingServiceADI,
	StakingServiceKeyBook,
	RequestsDataAccount,
	StateDataAccount,
	StakingServiceValidatorLiveAccount,
	DisputeAccount,
	Approved,
	Registered,
	ParametersUrl *url.URL

func init() {

	s := func(adi **url.URL, account string) {
		var err error
		if *adi, err = url.Parse(account); err != nil {
			panic(fmt.Sprintf("url to account is bad %v", err))
		}

	}

	s(&AccumulateStakingServiceADI, "acc://staking.acme")
	s(&StakingServiceKeyBook, "acc://staking.acme/StakingServiceKeyBook")
	s(&RequestsDataAccount, "acc://staking.acme/Requests")
	s(&StateDataAccount, "acc://staking.acme/State")
	s(&StakingServiceValidatorLiveAccount, "acc://staking.acme/Live")
	s(&DisputeAccount, "acc://staking.acme/Disputes")
	s(&Approved, "acc://staking.acme/Approved")
	s(&Registered, "acc://staking.acme/Registered")
	s(&ParametersUrl, "acc://staking.acme/Parameters")
}