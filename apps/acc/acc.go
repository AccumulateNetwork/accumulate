package acc

import (
	"fmt"
	"github.com/AccumulateNetwork/SMT/managed"
	"github.com/spf13/viper"
	"os"
	"os/signal"
	"syscall"

	//"github.com/AccumulateNetwork/accumulated/acc"
	"github.com/AccumulateNetwork/accumulated/tendermint"
)

const MaxNetworks = len(Networks)

var ConfigFile [MaxNetworks]string
var WorkingDir [MaxNetworks]string

//var SpecialModeHeight int64 = 99999999999

//SendToken GreenRock/t/acc RedWagon/t/acc 10
type SendToken struct {
	fromurl  string
	tourl    string
	actoshis uint64
}

type Fee struct {
	TimeStamp    int64        // 8
	DDII         managed.Hash // 32
	ChainID      [33]byte     // 33
	Credits      int8         // 1
	SignatureIdx int8         // 1
	Signature    []byte       // 64 minimum
	// 1 end byte ( 140 bytes for FEE)
	Transaction []byte // Transaction
}

func init() {
	//var name string
	//flag.StringVar(&name, "name", "", "your name")
	//flag.Parse()
	//values := flag.Args()
	//for _, word := range values {
	//	urlname := strings.ToLower(word)
	//	if !strings.HasPrefix("acc://", urlname) {
	//
	//	}
	//	if urlname != "" {
	//
	//		fmt.Println(strings.ToUpper(word))
	//	} else {
	//
	//		fmt.Println(word)
	//	}
	//}

	//if len(name) == 0 {
	//	fmt.Println("Usage: defaults.go -name")
	//	flag.PrintDefaults()
	//	os.Exit(1)
	//}
	//
	//
	//usr,err := user.Current()
	//if err != nil {
	//	panic( err )
	//	os.Exit(1)
	//}
	//
	////flag.StringVar(&WorkingDir[0], "workingdir", usr.HomeDir +  "/.accumulate", "Path to data directory")
	//flag.StringVar(&WorkingDir[0], "workingdir", usr.HomeDir +  "/.accumulate", "Path to data directory")
	//flag.Parse()
	////acc.Networks[0]
	//WorkingDir[1] = path.Join(WorkingDir[0],"/accumulate/rocky")
	//WorkingDir[2] = path.Join(WorkingDir[0],"/accumulate/smoky")
	//WorkingDir[3] = path.Join(WorkingDir[0],"/accumulate/yellowstone")
	//WorkingDir[0] = path.Join(WorkingDir[0],"/accumulate/leader")
	//ConfigFile[0] = path.Join(WorkingDir[0],"/config/config.toml")
	//ConfigFile[1] = path.Join(WorkingDir[1],"/config/config.toml")
	//ConfigFile[2] = path.Join(WorkingDir[2],"/config/config.toml")
	//ConfigFile[3] = path.Join(WorkingDir[3],"/config/config.toml")
}

func main() {

	fmt.Printf("Working dir: %v\n", WorkingDir[0])
	fmt.Printf("Working VM1 dir: %v\n", ConfigFile[1])
	fmt.Printf("Config File: %v\n", ConfigFile[0])
	fmt.Printf("Config File VM1: %v\n", ConfigFile[1])

	n := len(os.Args)
	for i := 0; i < n; i++ {
		switch os.Args[i] {
		case "init":
			tendermint.Initialize("accumulate.leader", "tcp://127.0.0.1:26600", "tcp://127.0.0.1:26601", "tcp://127.0.0.1:26602", "tcp://127.0.0.1:26603", "tcp://127.0.0.1:26604", ConfigFile[0], WorkingDir[0])
			tendermint.Initialize("accumulate.rocky", "tcp://127.0.0.1:26610", "tcp://127.0.0.1:26611", "tcp://127.0.0.1:26612", "tcp://127.0.0.1:26613", "tcp://127.0.0.1:26614", ConfigFile[1], WorkingDir[1])
			tendermint.Initialize("accumulate.smoky", "tcp://127.0.0.1:26620", "tcp://127.0.0.1:26621", "tcp://127.0.0.1:26622", "tcp://127.0.0.1:26623", "tcp://127.0.0.1:26624", ConfigFile[2], WorkingDir[2])
			tendermint.Initialize("accumulate.yellowstone", "tcp://127.0.0.1:26630", "tcp://127.0.0.1:26631", "tcp://127.0.0.1:26632", "tcp://127.0.0.1:26633", "tcp://127.0.0.1:26634", ConfigFile[3], WorkingDir[3])
			//			tendermint.Initialize("vm2", "tcp://127.0.0.1:26620","tcp://127.0.0.1:26621",ConfigFile[2],WorkingDir[2])
			//			tendermint.Initialize("vm3", "tcp://127.0.0.1:26630","tcp://127.0.0.1:26631",ConfigFile[3],WorkingDir[3])
			os.Exit(0)
		case "dbvc":

			os.Exit(0)
		}

	}

	//for now we'll assume
	viper.SetConfigFile(ConfigFile[1])
	viper.AddConfigPath(WorkingDir[1])
	viper.ReadInConfig()
	viper.GetString("accumulate.RouterAddress")

	//	accvm := CreateAccumulateVM(ConfigFile[1], WorkingDir[1])

	///we really need to open up ports to ALL shards in the system.  Maybe this should be a query to the DBVC blockchain.
	//	accvmapi, _ := accvm.GetAPIClient()
	//	urlrouter.AddShardClient(accvm.GetName(), accvmapi)

	//temporary server for each vm.  will be replaced by url router.
	//	go router.Jsonrpcserver2(accvmapi)
	//	urlrouter.AddShardClient("",accvmapi)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	os.Exit(0)
}
