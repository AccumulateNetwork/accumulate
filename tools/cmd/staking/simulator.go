package main

import (
	"fmt"
	"net/url"
	"sync"
	"time"
)

type Deposit struct {
	Amount     int64 // Amount of tokens deposited into the Account
	MajorBlock int64 // The major block when the deposit was made
}

type Account struct {
	URL      url.URL
	Deposits []*Deposit // List of deposits made into the Account
}

type Block struct {
	MajorHeight  int64
	MinorHeight  int64
	TokensIssued int64
	Timestamp    time.Time
}

type Simulator struct {
	mutex      sync.Mutex
	major,minor int64
	parameters *Parameters
	ADIs       []url.URL
	Accounts   []Account
	Blocks     []Block
}

type Parameters struct {
	ADI struct {
		StakingService *url.URL // Staking Service ADI acc://staking.acme
	}
	Account struct {
		TokenIssuance  *url.URL // Token Issuance account acc://acme
		RegisteredADIs *url.URL // RegisteredADIs staking acc://stacking.acme/Registered
		ApprovedADIs   *url.URL // ApprovedADIs staking acc://stacking.acme/Approved
		Disputes       *url.URL // Disputes of staking acc://staking.acme/Disputes
	}

	MajorTime         int64   // Time for a Major Block in seconds
	FirstPayOut       int64   // Block of the first Staking Payout
	PayOutFreq        int64   // Frequency of Payouts
	TokenLimit        int64   // Total Tokens to be issued
	TokenIssuanceRate float64 // APR of payout of unissued tokens
	StakingWeight     struct {
		PureStaking       float64 // Added weight for Pure Staking
		ProtocolValidator float64 // for Protocol Validators
		ProtocolFollower  float64 // for Protocol Followers
		StakingValidator  float64 // for Staking Validators
	}
	DelegateShare float64 // The share a Delegate Staker Keeps
	StakingShare  float64 // The share the Staker gets from the Delegates
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
	p.MajorTime = 5
	p.FirstPayOut = 4
	p.PayOutFreq = 4
	p.TokenLimit = 500e6
	p.TokenIssuanceRate = .16
	p.StakingWeight.PureStaking = 1.00
	p.StakingWeight.ProtocolValidator = 1.10
	p.StakingWeight.ProtocolFollower = 1.10
	p.StakingWeight.StakingValidator = 1.10
	p.DelegateShare = .95
	p.StakingShare = .05
}

func (s *Simulator) Init() {
	s.major = -1 
	s.minor = -1
	s.parameters = new(Parameters)
	s.parameters.Init()
}

func (s *Simulator) Run() {
	
	for {
		s.mutex.Lock()	
		 s.minor++
		if s.minor%s.parameters.MajorTime == 0 {
			s.major++
			fmt.Printf("\n%d :",s.major)
		}
		fmt.Printf(" %d",s.minor)
		s.mutex.Unlock()
		time.Sleep(time.Second)
	}

}