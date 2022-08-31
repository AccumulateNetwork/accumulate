package main

import (
	"fmt"
	"time"

	"go.uber.org/zap/buffer"
)

type StakingApp struct {
	Params *Parameters
	CBlk *Block
	Data struct {
		BlockHeight int64
		Timestamp string
		TokenLimit int64
		TokensIssued int64
		TokenIssuanceRate int64
		WeightPureSTaking int
		WeightProtocolValidator int
		WeightProtocolFollower int
		WeightStakingValidator int
		DelegatedAccountReturn int
		DelegateeShare int
	}
}

func(s *StakingApp) Collect() {
	s.Data.BlockHeight = s.CBlk.MajorHeight
	s.Data.Timestamp = s.CBlk.Timestamp.Format(time.RFC822)
	s.Data.TokenLimit = s.Params.TokenLimit
	s.Data.TokensIssued = s.CBlk.TokensIssued
	s.Data.TokenIssuanceRate = int64(s.Params.TokenIssuanceRate*100)
	s.Data.WeightPureSTaking = int(s.Params.StakingWeight.PureStaking*100)
	s.Data.WeightProtocolValidator = int(s.Params.StakingWeight.ProtocolValidator*100)
	s.Data.WeightProtocolFollower =int(s.Params.StakingWeight.ProtocolFollower*100)
	s.Data.WeightStakingValidator =int(s.Params.StakingWeight.StakingValidator*100)
	s.Data.DelegatedAccountReturn =int(s.Params.DelegateShare*100)
	s.Data.DelegateeShare =int(s.Params.DelegateeShare*100)
}


func (s *StakingApp) Run() {
	fmt.Println("Starting")
	sim := new(Simulator)
	sim.Init()
	go sim.Run()
	s.Params = sim.GetParameters()
	s.CBlk = sim.GetBlock()
	for {
		b := sim.GetBlock()
		if b.MajorHeight > s.CBlk.MajorHeight {
			s.CBlk = b
			c := (b.MajorHeight-s.Params.FirstPayOut)%s.Params.PayOutFreq 
			if c == 0 {
				s.Collect()
				s.Report()
			}
		}
		time.Sleep(time.Second/4)
	}
}

func (s *StakingApp) Report() {
	fmt.Println("Writing that Report for block ")
	report := new(buffer.Buffer)
	report.WriteString(fmt.Sprintf("\tMajor Block Height\t%d\n",s.Data.BlockHeight))
	report.WriteString(fmt.Sprintf("\tMajorBlockTimestamp\t%s\n",s.Data.Timestamp))
	fmt.Print(report.String())
}
