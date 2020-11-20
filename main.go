package main

import (
	//	"errors"
	"flag"
	"fmt"
	"log"

	// db "github.com/tendermint/tm-db"
	"path"
	//	"github.com/spf13/cobra"
	"os"
	"os/signal"
	"os/user"
	"syscall"

	//"github.com/dgraph-io/badger"
	"github.com/AccumulateNetwork/accumulated/tendermint"
	"github.com/AccumulateNetwork/accumulated/validator"
	"github.com/gorilla/mux"
	"github.com/gorilla/rpc"
	"github.com/gorilla/rpc/json"
	//"log"
	"net/http"

	//"net/rpc/jsonrpc"
)

var ConfigFile [33]string
var WorkingDir [33]string
//var SpecialModeHeight int64 = 99999999999

func init() {

	usr,err := user.Current()
	if err != nil {
		log.Fatal( err )
		os.Exit(1)
	}

	flag.StringVar(&WorkingDir[0], "workingdir", usr.HomeDir +  "/.accumulate", "Path to data directory")
	flag.Parse()
	WorkingDir[1] = path.Join(WorkingDir[0],"/valacc/fct")
	WorkingDir[2] = path.Join(WorkingDir[0],"/valacc/ec")
	WorkingDir[3] = path.Join(WorkingDir[0],"/valacc/user")
	WorkingDir[0] = path.Join(WorkingDir[0],"/leader")
	ConfigFile[0] = path.Join(WorkingDir[0],"/config/config.toml")
	ConfigFile[1] = path.Join(WorkingDir[1],"/config/config.toml")
	ConfigFile[2] = path.Join(WorkingDir[2],"/config/config.toml")
	ConfigFile[3] = path.Join(WorkingDir[3],"/config/config.toml")
}

type Args struct {
	A, B int
}
type Arith int
type Result int
func (t *Arith) Multiply(r *http.Request, args *Args, result *Result) error {
	log.Printf("Multiplying %d with %d\n", args.A, args.B)
	*result = Result(args.A * args.B)
	return nil
}

func jsonrpcserver () {
	s := rpc.NewServer()
	s.RegisterCodec(json.NewCodec(), "application/json")
	s.RegisterCodec(json.NewCodec(), "application/json;charset=UTF-8")
	arith := new(Arith)
	s.RegisterService(arith, "")
	r := mux.NewRouter()
	r.Handle("/rpc", s)
	go http.ListenAndServe(":1234", r)
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
			tendermint.Initialize("directory-block-leader", "tcp://127.0.0.1:26600","tcp://127.0.0.1:26601",ConfigFile[0],WorkingDir[0])
			tendermint.Initialize("accumulator-fct", "tcp://127.0.0.1:26610","tcp://127.0.0.1:26611",ConfigFile[1],WorkingDir[1])
			tendermint.Initialize("accumulator-ec", "tcp://127.0.0.1:26620","tcp://127.0.0.1:26621",ConfigFile[2],WorkingDir[2])
			tendermint.Initialize("accumulator-user", "tcp://127.0.0.1:26630","tcp://127.0.0.1:26631",ConfigFile[3],WorkingDir[3])
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
	factomvm := tendermint.NewAccumulatorVMApplication(ConfigFile[1],WorkingDir[1])
	factomvm.AddValidator(&val.ValidatorContext)

	go app.Start(ConfigFile[0],WorkingDir[0])
	go factomvm.Start()

	go jsonrpcserver()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	os.Exit(0)
}