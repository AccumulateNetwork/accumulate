package app

import (
	"fmt"
	"net/url"
	"time"
)

type Parameters struct {
	ADI struct {
		StakingService *url.URL // Staking Service ADI acc://staking.acme
	}
	Account struct {
		TokenIssuance  *url.URL // Token Issuance account acc://acme
		RegisteredADIs *url.URL // RegisteredADIs staking acc://stacking.acme/Registered
		ApprovedADIs   *url.URL // ApprovedADIs staking acc://stacking.acme/Approved
		Disputes       *url.URL // Disputes of staking acc://staking.acme/Disputes
		StakingConfig  *url.URL // Holds Parameter objects defining the config of staking
	}
	StartTime         time.Time     // This is time zero for the protocol
	MajorBlockTime    time.Duration // Scale time... 13000 gives about a 4 second major block
	PayOutFreq        int64         // Frequency of Payouts in major blocks. Only used in simulation
	TokenLimit        int64         // Total Tokens to be issued
	TokenIssuanceRate float64       // APR of payout of unissued tokens
	StakingWeight     struct {
		PureStaking       float64 // Added weight for Pure Staking
		ProtocolValidator float64 // for Protocol Validators
		ProtocolFollower  float64 // for Protocol Followers
		StakingValidator  float64 // for Staking Validators
	}
	DelegateShare  float64 // The share a Delegate Staker Keeps
	DelegateeShare float64 // The share the Staker gets from the Delegates
}

func (p *Parameters) String() string {
	return fmt.Sprintf("TokenLimit\t%d\n", p.TokenLimit) +
		fmt.Sprintf("TokenIssuanceRate\t%2f%%\n", p.TokenIssuanceRate*100) +
		"------------------------------------------------------------------------------------\n" +
		fmt.Sprintf("Pure Staking\t%3f%%\n", p.StakingWeight.PureStaking) +
		fmt.Sprintf("Protocol Validator\t%3f%%\n", p.StakingWeight.ProtocolValidator) +
		fmt.Sprintf("Protocol Follower\t%3f%%\n", p.StakingWeight.ProtocolFollower) +
		fmt.Sprintf("Staking Validator\t%3f%%\n", p.StakingWeight.StakingValidator)
}

// init()
// Initialize stuff in the package
// Initialize the starting values for Parameters
func (p *Parameters) Init() {
	var err1, err2, err3, err4, err5 error
	p.ADI.StakingService, err1 = url.Parse("acc://Staking.acme")
	p.Account.TokenIssuance, err2 = url.Parse("acc://acme")
	p.Account.RegisteredADIs, err3 = url.Parse("acc://staking.acme/Registered")
	p.Account.ApprovedADIs, err4 = url.Parse("acc://staking.acme/Approved")
	p.Account.Disputes, err5 = url.Parse("acc://staking.acme/Disputes")
	switch {
	case err1 != nil:
		panic(err1)
	case err2 != nil:
		panic(err1)
	case err3 != nil:
		panic(err1)
	case err4 != nil:
		panic(err1)
	case err5 != nil:
		panic(err1)
	}

	t := time.Now().UTC()                            // Get current time in UTC
	hours, minutes, seconds := t.Clock()             // Get our offset into the day, UTC
	t = t.Add(time.Duration(-hours) * time.Hour)     // Back t up by the hours
	t = t.Add(time.Duration(-minutes) * time.Minute) // and by the minutes
	t = t.Add(time.Duration(-seconds) * time.Second) // and by the seconds
	p.StartTime = t                                  // Start time at the start of today
	p.MajorBlockTime = time.Second /4                  // Should be 1 for the real app, ~13000 for testing
	p.PayOutFreq = 4                                 // The frequency at which payouts are made. Only used by simulation
	p.TokenLimit = 500e6                             // The Token limit for the protocol
	p.TokenIssuanceRate = .16                        // The percentage of unissued tokens paid out per year (limit - issued)
	p.StakingWeight.PureStaking = 1.00               // Weight given to Pure Staking accounts
	p.StakingWeight.ProtocolValidator = 1.10         // Weight given to Protocol Validator accounts
	p.StakingWeight.ProtocolFollower = 1.10          // Weight given to Protocol Follower accounts
	p.StakingWeight.StakingValidator = 1.10          // Weight given to Staking Validator accounts
	p.DelegateShare = .95                            // The share of tokens earned by delegation that delegates keep
	p.DelegateeShare = .05                           // The share of tokens earned by delegation that delegatees keep
}
