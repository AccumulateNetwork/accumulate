package app

import (
	"fmt"
	"log"
	"sort"
	"time"
)

var ReportDirectory string // The Directory where reports are written

// StakingApp
// The state of the Staking app, which is built up by catching up with
// the blocks in Accumulate.
type StakingApp struct {
	Params   *Parameters    // Staking Application Parameters
	PBlk     *Block         // Previous Block
	CBlk     *Block         // The current block being processed
	Accounts map[string]int // All the accounts we are tracking in the Staking App
	Data     struct {
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
		Type            string                   // The type of distribution
		AllAccounts     map[string]*Account      // Registered ADIs (Registered ADIs can add new staking accounts)
		Pure            []*Account               // Pure Stakers
		PValidator      []*Account               // Protocol Validators
		PFollower       []*Account               // Protocol Followers
		SValidator      []*Account               // Staking Validators
		Distributions   []*Distribution          // List of distributions to URLs
		DistributionMap map[string]*Distribution // A map of Delegate Distributions
	}
}

// Distribution
// Tracks the cells where distributions are calculated in the report so they can be referenced later
// when building token issuance transactions, and account reports
type Distribution struct {
	Tokens  int      // The row of the cell that defines the token distribution
	Account *Account // The url of the token account being paid
}

// Log
// Gives visual feedback to the user as the Staking application progresses.
func (s *StakingApp) Log(title string) {
	h, m, _ := s.CBlk.Timestamp.Clock()
	h2, m2, _ := s.CBlk.Timestamp.Local().Clock()
	fmt.Printf("%30s %5d %s %2d:%02d UTC -- %s %2d:%02d Local\n",
		title,
		s.CBlk.MajorHeight,
		s.CBlk.Timestamp.Format("Mon Jan 02 2006"),
		h, m,
		s.CBlk.Timestamp.Local().Format("Mon Jan 02 2006"),
		h2, m2)
}

// Run
// The main loop for the Staking application. It starts the simulator in the background
// for now.  Ultimately it will take a parameter on the command line to choose between
// the main net, the test net, and the simulator
func (s *StakingApp) Run(protocol Accumulate) {
	s.Accounts = make(map[string]int)  // Allocate the map of accounts we want to collect in a block
	s.Accounts[Approved.String()] = 1  // We watch the Approved data account, which has stakers
	s.protocol = protocol              // save away the protocol generating data
	p, err := protocol.GetParameters() // Get the first set of Parameters
	if err != nil {                    // Of course this should never happen
		log.Fatal("failed to get initial parameters")
		return
	}
	s.Params = p                                      // Set the initial set of  protocol parameters
	go protocol.Run()                                 // Run the processes
	s.Stakers.AllAccounts = make(map[string]*Account) // Track all the staking accounts
	for s.CBlk == nil {
		s.CBlk, err = s.protocol.GetBlock(1, s.Accounts)
		if err != nil {
			fmt.Print(".")
		}
		time.Sleep(time.Second)
	}
	s.Log("Starting")

	for i := int64(1); true; {
		b, err := s.GetBlock(i, s.Accounts)
		if err != nil || b == nil {
			time.Sleep(s.Params.MajorBlockTime / 12 / 60)
			continue
		}
		i++
		s.CBlk = b // This is the new current block
		s.ComputeBudget()
		s.AddAccounts()
		s.AddApproved(b)
		s.Report()
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
	var err error
	s.Data.TokensIssued, err = s.protocol.GetTokensIssued()
	if err != nil {
		log.Fatal(err)
	}
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

	switch approved.Type {
	case PureStaker:
		s.Stakers.Pure = append(s.Stakers.Pure, approved)
	case ProtocolValidator:
		s.Stakers.PValidator = append(s.Stakers.PValidator, approved)
	case ProtocolFollower:
		s.Stakers.PFollower = append(s.Stakers.PFollower, approved)
	case StakingValidator:
		s.Stakers.SValidator = append(s.Stakers.SValidator, approved)
	default:
		panic(fmt.Sprintf("Unknown account type: %v", approved.Type))
	}

	sort.Slice(s.Stakers.Pure, func(i, j int) bool { return s.Stakers.Pure[i].URL.String() < s.Stakers.Pure[j].URL.String() })
	sort.Slice(s.Stakers.PValidator, func(i, j int) bool {
		return s.Stakers.PValidator[i].URL.String() < s.Stakers.PValidator[j].URL.String()
	})
	sort.Slice(s.Stakers.PFollower, func(i, j int) bool { return s.Stakers.PFollower[i].URL.String() < s.Stakers.PFollower[j].URL.String() })
	sort.Slice(s.Stakers.SValidator, func(i, j int) bool {
		return s.Stakers.SValidator[i].URL.String() < s.Stakers.SValidator[j].URL.String()
	})
}

// GetBlock
// Used to check if we need to do staking processing in the block as we get
// them.  Note that GetBlock MUST be called in order by the Staking App.
// Because the order of the timestamps lets us detect the first of the month,
// and Friday. If the protocol stalls over a friday, then no rewards are
// paid out to stakers.
func (s *StakingApp) GetBlock(idx int64, accounts map[string]int) (*Block, error) {
	blk, err := s.protocol.GetBlock(idx, accounts)

	if blk != nil && idx == 1 {
		blk.SetBudget = true
		blk.PrintReport = false
		blk.PrintPayoutScript = false
		return blk, err
	}

	if blk == nil {
		return nil, nil
	}

	s.PBlk = s.CBlk
	s.CBlk = blk

	pMonth := s.PBlk.Timestamp.UTC().Month()       // Always compute a budget on the first
	month := s.CBlk.Timestamp.UTC().Month()        // major block of a new month.
	if s.CBlk.MajorHeight > 1 && pMonth != month { // The month changed. Compute a budget!
		s.CBlk.SetBudget = true
	}
	// The payday starts when the last block on Thursday completes.
	pDay := s.PBlk.Timestamp.UTC().Weekday()
	cDay := s.CBlk.Timestamp.UTC().Weekday()
	if idx > 7 && cDay == 5 && pDay < 5 {
		blk.PrintReport = true
	}
	// The script is printed in the major block after the report is produced
	if idx > 13 && s.PBlk.PrintReport { // (note that we always have a PBlk if idx>1)
		s.CBlk.PrintPayoutScript = true
	}

	return blk, nil
}
