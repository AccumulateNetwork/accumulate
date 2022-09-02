package main

import (
	"fmt"
	"math/rand"
	"net/url"
	"sync"
	"time"
)

type Deposit struct {
	MajorBlock int64 // The major block when the deposit was made
	Amount     int64 // Amount of tokens deposited into the Account
}

type UrlEntry struct {
	MajorBlock int64 // The major block when the URL was recorded
	Url        url.URL
}

type Account struct {
	MajorBlock int64         // The major block when Account was created
	URL        *url.URL      // The URL of this account
	Entries    []interface{} // Returns some struct from the account
	Type       string        // Type of account
	Balance    int64         // Balance if this is a token account
	Delegates  []*Account    // If this is a staker, and it has delegates
}

type Simulator struct {
	mutex        sync.Mutex          // Handle concurrent access
	major, minor int64               // Current block height
	parameters   *Parameters         //
	TokensIssued int64               // Total Tokens Issued
	ADIs         map[string]*url.URL //
	Accounts     map[string]*Account //
	Blocks       []*Block            //
	CurrentBlock *Block              //
}

func (s *Simulator) Init() {
	s.major = -1
	s.minor = -1
	s.parameters = new(Parameters)
	s.parameters.Init()
	s.CurrentBlock = new(Block)
	s.CurrentBlock.Timestamp = time.Now()
	s.TokensIssued = 203e6

	// Add some staking accounts
	a := new(Account)
	a.MajorBlock = 0
	a.URL = s.parameters.Account.RegisteredADIs
	for i := 0; i < 15; i++ {
		newAccount := new(Account)
		_, accountUrl := GenUrls("StakingAccount")
		newAccount.URL = accountUrl
		switch i % 4 {
		case 0:
			newAccount.Type = PureStaker
		case 1:
			newAccount.Type = ProtocolValidator
		case 2:
			newAccount.Type = ProtocolFollower
		case 3:
			newAccount.Type = ProtocolFollower
		}
		newAccount.Balance=rand.Int63()%10000000+1000000
		a.Entries = append(a.Entries, newAccount)
		s.CurrentBlock.Accounts = append(s.CurrentBlock.Accounts, a)
	}
}

func (s *Simulator) Run() {
	for {
		s.mutex.Lock()
		s.minor++
		if s.minor%s.parameters.MajorTime == 0 {
			s.major++
			fmt.Printf("\n%d :", s.major)
		}
		fmt.Printf(" %d", s.minor)

		s.Blocks = append(s.Blocks, s.CurrentBlock)
		s.CurrentBlock = new(Block)
		s.CurrentBlock.MinorHeight = s.minor
		s.CurrentBlock.MajorHeight = s.major
		s.CurrentBlock.Timestamp = time.Now()
		s.mutex.Unlock()
		time.Sleep(time.Second)
	}
}
