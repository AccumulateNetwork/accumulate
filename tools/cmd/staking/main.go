package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/user"
	"path"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/staking/app"
	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/staking/network"
	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/staking/sim"
)

var flagDebug = flag.Bool("debug", false, "Debug API requests")
var flagSim = flag.Bool("sim", false, "Use the simulator")
var strNet = flag.String("net", "https://testnet.accumulatenetwork.io/v2", "The network to run against")

func main() {
	flag.Parse()

	u, _ := user.Current()
	app.ReportDirectory = path.Join(u.HomeDir + "/StakingReports")
	if err := os.MkdirAll(app.ReportDirectory, os.ModePerm); err != nil {
		fmt.Println(err)
		return
	}
	
	s := new(app.StakingApp)
	if *flagSim {
		sim := new(sim.Simulator)
		s.Run(sim)
		return
	}

	net, err := network.New(*strNet, protocol.AccountUrl("staking.acme", "parameters"))
	if err != nil {
		log.Fatal(err)
	}
	if *flagDebug {
		net.Debug()
	}
	s.Run(net)
}
