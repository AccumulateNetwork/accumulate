package app

import (
	"fmt"
	"sort"
	"time"
)

var ReportDirectory string

// StakingApp
// The state of the Staking app, which is built up by catching up with
// the blocks in Accumulate.
type StakingApp struct {
	Params *Parameters
	CBlk   *Block
	Data   struct {
		BlockHeight            int64
		Timestamp              string
		TokenLimit             int64
		TokensIssued           int64
		TokenIssuanceRate      int64
		WtPS                   int64
		WtPV                   int64
		WtPF                   int64
		WtSV                   int64
		DelegatedAccountReturn int64
		DelegateeShare         int64
		TotalTokens            int64 // Total staked tokens
		TotalWeighted          int64 // Total weighted tokens
		TotalPS                int64 // Total Pure Staking Tokens
		TotalPV                int64 // Total Protocol Validator Tokens
		TotalPF                int64 // Total Protocol Follower Tokens
		TotalSV                int64 // Total Staking Validator Tokens
		TotalPSD               int64 // Total Pure Staking Tokens + delegate Tokens
		TotalPVD               int64 // Total Protocol Validator Tokens + delegate Tokens
		TotalPFD               int64 // Total Protocol Follower Tokens + delegate Tokens
		TotalSVD               int64 // Total Staking Validator Tokens + delegate Tokens
		PSS                    int64 // Pure Staking Start
		PSE                    int64 // Pure Staking End
		PVS                    int64 // Protocol Validator Start
		PVE                    int64 // Protocol Validator End
		PFS                    int64 // Protocol Follower Start
		PFE                    int64 // Protocol Follower End
		SVS                    int64 // Staking Validator Start
		SVE                    int64 // Staking Validator End
	}
	protocol Accumulate
	Stakers  struct {
		AllAccounts map[string]*Account // Registered ADIs (Registered ADIs can add new staking accounts)
		Pure        []*Account          // Pure Stakers
		PValidator  []*Account          // Protocol Validators
		PFollower   []*Account          // Protocol Followers
		SValidator  []*Account          // Staking Validators
	}
}

// Log
// Gives visual feedback to the user as the Staking application progresses.
func (s *StakingApp) Log(title string) {
	h, m, _ := s.CBlk.Timestamp.Clock()
	h2, m2, _ := s.CBlk.Timestamp.Local().Clock()
	fmt.Printf("%30s %5d %s %2d:%02d UTC -- %s %2d:%02d Local\n",
		title,
		s.CBlk.MajorHeight,
		s.CBlk.Timestamp.Format("Mon 02-Jan-06"),
		h, m,
		s.CBlk.Timestamp.Local().Format("Mon 02-Jan-06"),
		h2, m2)
}

// Run
// The main loop for the Staking application. It starts the simulator in the background
// for now.  Ultimately it will take a parameter on the command line to choose between
// the main net, the test net, and the simulator
func (s *StakingApp) Run(protocol Accumulate) {
	s.protocol = protocol
	s.protocol.Init()
	go protocol.Run()
	s.Params = protocol.GetParameters()
	s.Stakers.AllAccounts = make(map[string]*Account)

	for s.CBlk == nil {
		s.CBlk = s.protocol.GetBlock(0)
		time.Sleep(time.Second)
	}
	s.Log("Starting")

	for i := int64(0); true; {
		b := s.protocol.GetBlock(i)
		if b == nil {
			time.Sleep(s.Params.MajorBlockTime / 12 / 60)
			continue
		}
		i++
		s.CBlk = b // This is the new current block
		s.ComputeBudget()
		s.AddAccounts()
		s.AddApproved(b)
		s.Report()
		s.PrintPayScript()
	}
}

// ComputeBudget()
// On the first day of every month, the budget for distribution to stakers
// is calculated.  This calculates the weekly budget, which is distributed to
// the stakers every Friday for staking that occurred in the previous week.
// Partial weeks do not receive rewards.
func (s *StakingApp) ComputeBudget() {
	if !s.CBlk.SetBudget {
		return
	}
	s.Log("Set Monthly Budget")
	s.Data.TokensIssued = s.protocol.GetTokensIssued()
	s.Data.TokenIssuanceRate = int64(s.Params.TokenIssuanceRate * 100)

}

// Collect
// Collect all the data to be used within the Report.  Most of this data is
// take from the state of the Staking Application, which has been running
// all along.
func (s *StakingApp) Collect() {
	if !s.CBlk.PrintReport {
		return
	}
	s.Data.BlockHeight = s.CBlk.MajorHeight
	s.Data.Timestamp = s.CBlk.Timestamp.UTC().Format(time.UnixDate)
	s.Data.TokenLimit = s.Params.TokenLimit
	s.Data.WtPS = int64(s.Params.StakingWeight.PureStaking * 100)
	s.Data.WtPV = int64(s.Params.StakingWeight.ProtocolValidator * 100)
	s.Data.WtPF = int64(s.Params.StakingWeight.ProtocolFollower * 100)
	s.Data.WtSV = int64(s.Params.StakingWeight.StakingValidator * 100)
	s.Data.DelegatedAccountReturn = int64(s.Params.DelegateShare * 100)
	s.Data.DelegateeShare = int64(s.Params.DelegateeShare * 100)

	s.Data.TotalPS, s.Data.TotalPSD = TotalAccounts(s.Stakers.Pure)
	s.Data.TotalPV, s.Data.TotalPVD = TotalAccounts(s.Stakers.PValidator)
	s.Data.TotalPF, s.Data.TotalPFD = TotalAccounts(s.Stakers.PFollower)
	s.Data.TotalSV, s.Data.TotalSVD = TotalAccounts(s.Stakers.SValidator)
	s.Data.TotalTokens = s.Data.TotalPS + s.Data.TotalPV + s.Data.TotalPF + s.Data.TotalSV
	s.Data.TotalWeighted = int64(float64(s.Data.TotalPSD)*s.Params.StakingWeight.PureStaking +
		float64(s.Data.TotalPVD)*s.Params.StakingWeight.ProtocolValidator +
		float64(s.Data.TotalPFD)*s.Params.StakingWeight.ProtocolFollower +
		float64(s.Data.TotalSVD)*s.Params.StakingWeight.StakingValidator)
}

// AddApproved
// Look in the block at the Approved Account, and add any new entries
func (s *StakingApp) AddApproved(b *Block) {
	approved := b.GetAccount(Approved)
	if approved == nil {
		return
	}
	for _, v := range approved.Entries {
		account := v.(*Account)
		switch account.Type {
		case PureStaker:
			s.Stakers.Pure = append(s.Stakers.Pure, account)
		case ProtocolValidator:
			s.Stakers.PValidator = append(s.Stakers.PValidator, account)
		case ProtocolFollower:
			s.Stakers.PFollower = append(s.Stakers.PFollower, account)
		case StakingValidator:
			s.Stakers.SValidator = append(s.Stakers.SValidator, account)
		default:
			panic(fmt.Sprintf("Unknown account type: %v", account.Type))
		}
	}
	sort.Slice(s.Stakers.Pure, func(i, j int) bool { return s.Stakers.Pure[i].URL.String() < s.Stakers.Pure[j].URL.String() })
	sort.Slice(s.Stakers.Pure, func(i, j int) bool { return s.Stakers.Pure[i].URL.String() < s.Stakers.Pure[j].URL.String() })
	sort.Slice(s.Stakers.Pure, func(i, j int) bool { return s.Stakers.Pure[i].URL.String() < s.Stakers.Pure[j].URL.String() })
	sort.Slice(s.Stakers.Pure, func(i, j int) bool { return s.Stakers.Pure[i].URL.String() < s.Stakers.Pure[j].URL.String() })
}

// PrintPayScript
// Write out a script to call the CLI to Distribute tokens, or to sign the Distribution of Tokens
func (s *StakingApp) PrintPayScript() {
	if s.CBlk.PrintPayoutScript {
		s.Log("PayScript")
	}
}
