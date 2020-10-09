package main

import (
	//	"errors"
	"flag"
	"fmt"

	// db "github.com/tendermint/tm-db"
	"path"
	//	"github.com/spf13/cobra"
	"os"
	"os/signal"
	"syscall"

	//"github.com/dgraph-io/badger"
	"github.com/AccumulusNetwork/accumulated/tendermint"
	"github.com/AccumulusNetwork/accumulated/validator"
)

var ConfigFile [33]string
var WorkingDir [33]string
//var SpecialModeHeight int64 = 99999999999

func init() {
	flag.StringVar(&WorkingDir[0], "workingdir", "$HOME/.accumulus", "Path to data directory")
	WorkingDir[1] = path.Join(WorkingDir[1],"/vm/1")
	WorkingDir[2] = path.Join(WorkingDir[2],"/vm/2")
	WorkingDir[3] = path.Join(WorkingDir[3],"/vm/3")
	WorkingDir[0] = path.Join(WorkingDir[0],"/dirblock")
	flag.Parse()
	ConfigFile[0] = path.Join(WorkingDir[0],"/config/config.toml")
	ConfigFile[1] = path.Join(WorkingDir[1],"/config/config.toml")
	ConfigFile[2] = path.Join(WorkingDir[2],"/config/config.toml")
	ConfigFile[3] = path.Join(WorkingDir[3],"/config/config.toml")
}

func main() {

	fmt.Printf("Working dir: %v\n", WorkingDir[0])
	fmt.Printf("Working VM1 dir: %v\n", ConfigFile[1])
	fmt.Printf("Config File: %v\n", ConfigFile[0])
	fmt.Printf("Config File VM1: %v\n", ConfigFile[1])

	n := len(os.Args)
	for i := 0; i<n; i++ {
    	switch os.Args[i] {
		case "init":
			tendermint.Initialize("dirblock", "tcp://127.0.0.1:26600","tcp://127.0.0.1:26601",ConfigFile[0],WorkingDir[0])
			tendermint.Initialize("vm1", "tcp://127.0.0.1:26610","tcp://127.0.0.1:26611",ConfigFile[1],WorkingDir[1])
//			tendermint.Initialize("vm2", "tcp://127.0.0.1:26620","tcp://127.0.0.1:26621",ConfigFile[2],WorkingDir[2])
//			tendermint.Initialize("vm3", "tcp://127.0.0.1:26630","tcp://127.0.0.1:26631",ConfigFile[3],WorkingDir[3])
			os.Exit(0)
		}
	}


	//create a directory block leader
	app := tendermint.NewDirectoryBlockLeader()

	//make a factoid validator/accumulator
	//create a Factoid validator
	val := validator.NewFactoidValidator()
	//create a AccumulatorVM
	factoidvm := tendermint.NewAccumulatorVMApplication(val)

	go app.Start(ConfigFile[0],WorkingDir[0])
	go factoidvm.Start(ConfigFile[1],WorkingDir[1])

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	os.Exit(0)
}