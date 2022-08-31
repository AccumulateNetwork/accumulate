package main

import (
	"time"

	
)

type StakingApp struct {
	acc         Accumulate // Our interface to Accumulate
	PayoutBlock int64      // The next Payout Block
}

func (s *StakingApp) Run() {
	sim := new(Simulate)
	
	sim.App = s
	
	s.acc.Start()
	for {
		time.Sleep(time.Second)
	}
}

func (s *StakingApp) Report() {
	//report := new(buffer.Buffer)
	//report.WriteString(fmt.Sprintf("\tMajor Block Height\t%d\n",major))
	//report.WriteString(fmt.Sprintf("\tMajorBlockTimestamp\t%s\n",timestamp))
}