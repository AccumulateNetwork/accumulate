package main

import "net/url"

const (
	PureStaker        = "Pure Staker"
	ProtocolValidator = "Protocol Validator"
	ProtocolFollower  = "Protocol Follower"
	StakingValidator  = "Staking Validator"
	Delegate          = "Delegate"

	SecondsPerMajor = 5
)

type Accumulate interface {
	GetBlock(index int) *Block // Get the Major and Minor block heights
	Start()                    // Any initialization required for the interface
}

type StakingADI struct {
	Activation int64         // Block where Staker is activated
	AdiUrl     *url.URL      // The Staker's ADI
	AccountUrl *url.URL      // URL of the Staking Account
	Type       string        // Type of validator
	StakerADI  *url.URL      // ADI of the staking account (if delegated)
	Delegates  []*StakingADI // List of Delegates (if not a delegate)
}
