package main

import (
	"net/url"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/smt/common"
)

type Deposit struct {
	MajorBlock int64 // The major block when the deposit was made
	Amount     int64 // Amount of tokens deposited into the Account
}

type UrlEntry struct {
	MajorBlock int64 // The major block when the URL was recorded
	Url        url.URL
}

type Registration struct {
	ADI        *url.URL // The URL of the registered ADI
	MajorBlock int64    // The Major Block where the ADI was registered
	Type       []string // The Types for which this ADI is registered
}

type Account struct {
	MajorBlock int64         // The Major Block where Account was approved
	URL        *url.URL      // The URL of this account
	Entries    []interface{} // Returns some struct from the account
	Type       string        // Type of account
	Delegatee  *Account      // If this is a delegate, Account it delegates to.
	Balance    int64         // Balance if this is a token account
	Delegates  []*Account    // If this is a staker, and it has delegates
}

type Simulator struct {
	mutex        sync.Mutex          // Handle concurrent access
	major        int64               // Current block height
	parameters   *Parameters         //
	TokensIssued int64               // Total Tokens Issued
	ADIs         map[string]*url.URL //
	Accounts     map[string]*Account //
	MajorBlocks  []*Block            // List of Major Blocks
}

var rh common.RandHash // A random Series for just setting up accounts

func (s *Simulator) Init() {
	s.major = 0
	s.parameters = new(Parameters)
	s.parameters.Init()
	s.TokensIssued = 203e6

	// Add some staking accounts
	registered := new(Account)
	registered.MajorBlock = 0
	registered.URL = s.parameters.Account.RegisteredADIs
	var last *Account
	idx := int64(0) // Make the first account type a PureStaker
	var issued int64
	for {
		newAccount := new(Account)
		_, accountUrl := GenUrls("StakingAccount")
		newAccount.URL = accountUrl
		switch idx {
		case 0:
			newAccount.Type = PureStaker
			last = newAccount
		case 1:
			newAccount.Type = ProtocolValidator
			last = newAccount
		case 2:
			newAccount.Type = ProtocolFollower
			last = newAccount
		case 3:
			newAccount.Type = StakingValidator
			last = newAccount
		case 4, 5, 6:
			newAccount.Type = Delegate
			newAccount.Delegatee = last
		}
		newAccount.Balance = rh.GetRandInt64()%13000000 + 25000
		if idx > 3 { //                If a delegate...
			newAccount.Balance /= 100 // Delegates generally have lower stake.
		}
		// Run until we have something less than 200 million tokens staked.
		if issued+newAccount.Balance > 200000000 {
			break
		}
		issued += newAccount.Balance
		registered.Entries = append(registered.Entries, newAccount)
		idx = rh.GetRandInt64() % 7 // Figure out the next staking type
	}
	s.Accounts = make(map[string]*Account)
	s.Accounts[registered.URL.String()] = registered
}

func (s *Simulator) Run() {
	s.mutex.Lock()                 // Going to update the simulator
	blk := new(Block)              // Create the first block
	for _, v := range s.Accounts { // Add all the accounts
		blk.Accounts = append(blk.Accounts, v)
	}
	s.mutex.Unlock()

	for {
		time.Sleep(time.Second)                    // Sleep while unlocked
		s.mutex.Lock()                             // Lock
		s.MajorBlocks = append(s.MajorBlocks, blk) // Add it to the block list
		s.major++                                  // Add the current major block
		blk = new(Block)                           // Create the next block
		blk.MajorHeight = s.major                  // Set the major block number
		blk.Timestamp = time.Now()                 // Set the timestamp
		s.mutex.Unlock()                           // Unlock to allow others access to simulator state
	}
}
