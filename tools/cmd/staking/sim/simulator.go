package sim

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/staking/app"
)

var _ = fmt.Printf

// Simulator
// This is
type Simulator struct {
	mutex        sync.Mutex              // Handle concurrent access
	major        int64                   // Current block height
	parameters   *app.Parameters         //
	tokensIssued int64                   // Total Tokens Issued
	ADIs         map[string]*url.URL     //
	Accounts     map[string]*app.Account //
	MajorBlocks  []*app.Block            // List of Major Blocks
	CBlk         *app.Block              // Current Block under construction
}

var _ app.Accumulate = (*Simulator)(nil)

var rh common.RandHash // A random Series for just setting up accounts

func (s *Simulator) TokensIssued(numTokens int64) {
	s.tokensIssued = numTokens
}

func (s *Simulator) Init() {
	s.major = 0
	s.parameters = new(app.Parameters)
	s.parameters.Init()
	s.tokensIssued = 203e6

	// Add some staking accounts
	registered := new(app.Account)
	registered.MajorBlock = 0
	registered.URL = s.parameters.Account.RegisteredADIs
	var last *app.Account
	idx := int64(0) // Make the first account type a PureStaker
	var issued int64
	for {
		newAccount := new(app.Account)
		_, accountUrl := GenUrls("StakingAccount")
		newAccount.URL = accountUrl
		newAccount.DepositURL = accountUrl
		if rh.GetRandInt64()%100 > 60 {
			var err error
			hostname := accountUrl.Hostname()
			Scheme := accountUrl.Scheme
			depositUrl := Scheme + "://" + hostname + "/Rewards"
			if newAccount.DepositURL, err = url.Parse(depositUrl); err != nil {
				panic(err)
			}
		}
		switch idx {
		case 0:
			newAccount.Type = app.PureStaker
			last = newAccount
		case 1:
			newAccount.Type = app.ProtocolValidator
			last = newAccount
		case 2:
			newAccount.Type = app.ProtocolFollower
			last = newAccount
		case 3:
			newAccount.Type = app.StakingValidator
			last = newAccount
		case 4, 5, 6:
			newAccount.Type = app.Delegate
			newAccount.Delegatee = last
		}
		newAccount.Balance = rh.GetRandInt64()%13000000 + 25000
		if idx > 3 { //                  If a delegate...
			newAccount.Balance /= 50 // Delegates generally have lower stake.
		}
		// Run until we have something less than 200 million tokens staked.
		if issued+newAccount.Balance > 150000000 {
			break
		}
		issued += newAccount.Balance

		registered.Entries = append(registered.Entries, newAccount)
		idx = rh.GetRandInt64() % 7 // Figure out the next staking type
	}
	s.Accounts = make(map[string]*app.Account)
	s.Accounts[registered.URL.String()] = registered
}

func (s *Simulator) Run() {
	s.mutex.Lock()                 // Going to update the simulator
	s.CBlk = new(app.Block)        // Create the first block
	for _, v := range s.Accounts { // Add all the accounts
		s.CBlk.Accounts = append(s.CBlk.Accounts, v)
	}
	s.mutex.Unlock()

	for {
		time.Sleep(s.parameters.MajorBlockTime)
		s.mutex.Lock()                                // Lock
		s.CBlk.Timestamp = s.GetTime()                // Set the timestamp
		s.MajorBlocks = append(s.MajorBlocks, s.CBlk) // Add it to the block list
		s.CBlk = new(app.Block)                       // Create the next block
		s.major++                                     // Add the current major block
		s.CBlk.MajorHeight = s.major                  // Set the major block number
		s.mutex.Unlock()                              // Unlock to allow others access to simulator state

	}
}

// GetTime
// Return scaled time, so we can process even years of blocks
func (s *Simulator) GetTime() time.Time {
	MBD := 12 * time.Hour                          // Major Block Duration
	NBlks := time.Duration(len(s.MajorBlocks))     // The Number of Blocks so far
	return s.parameters.StartTime.Add(MBD * NBlks) // Start Time + #blocks * block duration
}
