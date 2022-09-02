package main

import (
	"fmt"
	"os"
	"path"
	"sort"
	"time"

	"go.uber.org/zap/buffer"
)

type StakingApp struct {
	Params *Parameters
	CBlk   *Block
	Data   struct {
		BlockHeight             int64
		Timestamp               string
		TokenLimit              int64
		TokensIssued            int64
		TokenIssuanceRate       int64
		WeightPureSTaking       int
		WeightProtocolValidator int
		WeightProtocolFollower  int
		WeightStakingValidator  int
		DelegatedAccountReturn  int
		DelegateeShare          int
	}
	sim     *Simulator
	Stakers struct {
		Pure       []*Account // Pure Stakers
		PValidator []*Account // Protocol Validators
		PFollower  []*Account // Protocol Followers
		SValidator []*Account // Staking Validators
	}
	
}

func (s *StakingApp) Collect() {
	s.Data.BlockHeight = s.CBlk.MajorHeight
	s.Data.Timestamp = s.CBlk.Timestamp.UTC().Format(time.UnixDate)
	s.Data.TokenLimit = s.Params.TokenLimit
	s.Data.TokensIssued = s.sim.GetTokensIssued()
	s.Data.TokenIssuanceRate = int64(s.Params.TokenIssuanceRate * 100)
	s.Data.WeightPureSTaking = int(s.Params.StakingWeight.PureStaking * 100)
	s.Data.WeightProtocolValidator = int(s.Params.StakingWeight.ProtocolValidator * 100)
	s.Data.WeightProtocolFollower = int(s.Params.StakingWeight.ProtocolFollower * 100)
	s.Data.WeightStakingValidator = int(s.Params.StakingWeight.StakingValidator * 100)
	s.Data.DelegatedAccountReturn = int(s.Params.DelegateShare * 100)
	s.Data.DelegateeShare = int(s.Params.DelegateeShare * 100)
}

func (s *StakingApp) Run() {
	fmt.Println("Starting")
	sim := new(Simulator)
	s.sim = sim
	sim.Init()
	go sim.Run()
	s.Params = sim.GetParameters()
	s.CBlk = sim.GetBlock()
	for {
		b := sim.GetBlock()
		if b.MajorHeight > s.CBlk.MajorHeight {
			s.CBlk = b
			c := (b.MajorHeight - s.Params.FirstPayOut) % s.Params.PayOutFreq
			if c == 0 {
				s.Collect()
				s.Report()
			}
		}
		s.AddApproved(b)
		//s.SumRewards()
		time.Sleep(time.Second / 4)
	}
}

func (s *StakingApp) Report() {

	fmt.Print("\nWriting that Report for block\n\t.\n")
	report := new(buffer.Buffer)
	f := func(format string, a ...any) {
		report.WriteString(fmt.Sprintf(format, a...))
	}
	f("Report:\tMajor Block Height\t%d\n", s.Data.BlockHeight)               // 1
	f("\tMajorBlockTimestamp\t%s\n", s.Data.Timestamp)                       // 2
	f("\n")                                                                  // 3
	f("\tBudget\n")                                                          // 4
	f("\tACME Token limit\t%d\n", s.Data.TokenLimit)                         // 5
	f("\tACME Tokens Issued\t%d\n", s.Data.TokensIssued)                     // 6
	f("\tToken Issuance Rate\t%d%%\n", s.Data.TokenIssuanceRate)             // 7
	f("\tAnnual Budget\t=(C5-C6)*C7\n")                                      // 8
	f("\tWeekly Budget\t=int(C8/365.25*10*7)/10\n")                          // 9
	f("\tAPR\t=c8/c21\n")                                                    // 10
	f("\t\n")                                                                // 11
	f("\tWeight Pure Staking Account\t%d%%\n", s.Data.WeightPureSTaking)     // 12
	f("\tWeight Protocol Validator\t%d%%\n", s.Data.WeightProtocolValidator) // 13
	f("\tWeight Protocol Follower\t%d%%\n", s.Data.WeightProtocolFollower)   // 14
	f("\tWeight Staking Validator\t%d%%\n", s.Data.WeightStakingValidator)   // 15
	f("\n")                                                                  // 16
	f("\tDelegated Account Return\t%d%%\n", s.Data.DelegatedAccountReturn)   // 17
	f("\tStaking Share of Delegated Tokens\t%d%%\n", s.Data.DelegateeShare)  // 18
	f("")                                                                    // 19
	f("")                                                                    // 20
	f("")                                                                    // 21
	f("")                                                                    // 22
	f("")                                                                    // 23
	f("")                                                                    // 24
	f("")                                                                    // 25
	f("")                                                                    // 26
	f("")                                                                    // 27
	f("")                                                                    // 28
	f("")                                                                    // 29
	f("")                                                                    // 30
	fmt.Print(report.String())
	reportFile := path.Join(ReportDirectory, fmt.Sprintf("Report-%d.csv", s.Data.BlockHeight))
	if f, err := os.Create(reportFile); err != nil {
		panic(fmt.Sprintf("Could not create %s: %v", reportFile, err))
	} else {
		if _, err := f.Write(report.Bytes()); err != nil {
			panic(fmt.Sprintf("could not write to file %s: %v", reportFile, err))
		}
		if err := f.Close(); err != nil {
			panic(fmt.Sprintf("could not close file %s: %v", reportFile, err))
		}
	}
}

// AddApproved
// Look in the block at the Approved Account, and add any new entries
func (s *StakingApp) AddApproved(b *Block) {
	approved := b.GetAccount(ApprovedADIs)
	if approved == nil {
		return
	}
	for _, v := range approved.Entries {
		account := v.(*Account)
		switch account.Type {
		case PureStaker:
			s.Stakers.Pure = append(s.Stakers.Pure,account)	
		case ProtocolValidator:
			s.Stakers.PValidator = append(s.Stakers.PValidator,account)	
		case ProtocolFollower:
			s.Stakers.PFollower = append(s.Stakers.PFollower,account)	
		case StakingValidator:
			s.Stakers.SValidator = append(s.Stakers.SValidator,account)	
		default:
			panic(fmt.Sprintf("Unknown account type: %v",account.Type))			
		}
	}
	sort.Slice(s.Stakers.Pure,func(i,j int)bool{return s.Stakers.Pure[i].URL.String()<s.Stakers.Pure[j].URL.String()})
	sort.Slice(s.Stakers.Pure,func(i,j int)bool{return s.Stakers.Pure[i].URL.String()<s.Stakers.Pure[j].URL.String()})
	sort.Slice(s.Stakers.Pure,func(i,j int)bool{return s.Stakers.Pure[i].URL.String()<s.Stakers.Pure[j].URL.String()})
	sort.Slice(s.Stakers.Pure,func(i,j int)bool{return s.Stakers.Pure[i].URL.String()<s.Stakers.Pure[j].URL.String()})
}

