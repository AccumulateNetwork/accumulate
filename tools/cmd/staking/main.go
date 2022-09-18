package main

import (
	"fmt"
	"os"
	"os/user"
	"path"

	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/staking/app"
	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/staking/sim"
)

func main() {
	u, _ := user.Current()
	app.ReportDirectory = path.Join(u.HomeDir + "/StakingReports")
	if err := os.MkdirAll(app.ReportDirectory, os.ModePerm); err != nil {
		fmt.Println(err)
		return
	}

	s := new(app.StakingApp)
	sim := new(sim.Simulator)
	s.Run(sim)
}
