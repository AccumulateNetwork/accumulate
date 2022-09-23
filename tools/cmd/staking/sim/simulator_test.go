package sim

import (
	"fmt"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/staking/app"
)

func TestSimulator(t *testing.T) {
	sim := new(Simulator)
	sim.Init()
	go sim.Run()

	time.Sleep(11 * time.Second)
}

func TestSimulator_GetTime(t *testing.T) {
	s := new(Simulator)
	s.Init()
	for i := 0; i < 1000; i++ {
		s.CBlk = new(app.Block)
		s.MajorBlocks = append(s.MajorBlocks, s.CBlk)
		fmt.Printf("%s\n", s.GetTime().Format("Monday, 02-Jan-06 15:04:05 UTC"))
		time.Sleep(time.Second / 4)
	}

}
