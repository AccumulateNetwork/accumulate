package main

import (
	"github.com/AccumulateNetwork/accumulated/blockchain/accnode"
	"github.com/AccumulateNetwork/accumulated/router"
	"github.com/spf13/viper"

	//	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path"

	"os/signal"
	"os/user"
	"syscall"

	"github.com/AccumulateNetwork/accumulated/blockchain/tendermint"
)

var ConfigFile []string
var WorkingDir []string
const DBVCIndex = 0

var (
	BuildTag = "v0.0.1"
)
//var SpecialModeHeight int64 = 99999999999

func init() {

	usr,err := user.Current()
	if err != nil {
		log.Fatal( err )
		os.Exit(1)
	}
	initdir := path.Join(usr.HomeDir , "/.accumulate" )

	version := flag.Bool("v", false, "prints the current version")
	flag.StringVar(&initdir, "workingdir", usr.HomeDir +  "/.accumulate", "Path to data directory")
	flag.Parse()
	if *version {
		fmt.Printf("Accumulate BVC %s\n",BuildTag)
		os.Exit(0)
	}
	for i := range router.Networks {
		WorkingDir = append(WorkingDir, path.Join(initdir, router.Networks[i]))
		ConfigFile = append(ConfigFile, path.Join(WorkingDir[i],"/config/config.toml"))
	}

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
			baseport := 26600
			for i := range router.Networks {
				abciAppAddress := fmt.Sprintf("tcp://localhost:%d", baseport)
				rcpAddress := fmt.Sprintf("tcp://localhost:%d", baseport+1)
				grpcAddress := fmt.Sprintf("tcp://localhost:%d", baseport+2)
				accRCPAddress := fmt.Sprintf("tcp://localhost:%d", baseport+3)
				routerAddress := fmt.Sprintf("tcp://localhost:%d", baseport+4)
				baseport += 5
				tendermint.Initialize("accumulate." + router.Networks[i],abciAppAddress,rcpAddress,grpcAddress,accRCPAddress,routerAddress,ConfigFile[i],WorkingDir[i])
			}
			os.Exit(0)
		case "dbvc":
			os.Exit(0)
		}
	}


	//First create a router
	viper.SetConfigFile(ConfigFile[1])
	viper.AddConfigPath(WorkingDir[1])
	viper.ReadInConfig()
	urlrouter := router.NewRouter(viper.GetString("accumulate.RouterAddress"))

	//Next create a BVC
	accvm := accnode.CreateAccumulateBVC(ConfigFile[1], WorkingDir[1])

	///we really need to open up ports to ALL shards in the system.  Maybe this should be a query to the DBVC blockchain.
	accvmapi, _ := accvm.GetAPIClient()
	urlrouter.AddBVCClient(accvm.GetName(), accvmapi)

	//temporary server for each vm.  will be replaced by url router.
	//go router.Jsonrpcserver2(accvmapi)


	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	os.Exit(0)
}