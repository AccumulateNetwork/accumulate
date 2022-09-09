// File holds interfaces and constants

package main

import (
	"fmt"
	"net/url"
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
	GetBlock(index int) *Block // Get the Major and Minor block heights
	Start()                    // Any initialization required for the interface
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
	Registered *url.URL

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

}
