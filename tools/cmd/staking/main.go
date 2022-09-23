package main

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"path"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/staking/app"
	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/staking/network"
)

func main() {
	u, _ := user.Current()
	app.ReportDirectory = path.Join(u.HomeDir + "/StakingReports")
	if err := os.MkdirAll(app.ReportDirectory, os.ModePerm); err != nil {
		fmt.Println(err)
		return
	}

	s := new(app.StakingApp)
	net, err := network.New("https://testnet.accumulatenetwork.io/v2", protocol.AccountUrl("staking.acme", "parameters"))
	if err != nil {
		log.Fatal(err)
	}
	s.Run(net)
	// sim := new(sim.Simulator)
	// s.Run(sim)
}
