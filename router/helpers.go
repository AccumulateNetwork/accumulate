package router

import (
	"encoding/hex"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/api/proto"
	"github.com/AccumulateNetwork/accumulated/blockchain/accnode"
	"github.com/AccumulateNetwork/accumulated/blockchain/tendermint"
	"github.com/AccumulateNetwork/accumulated/blockchain/validator/types"
	"github.com/spf13/viper"
	"github.com/tendermint/tendermint/crypto/ed25519"
	tmnet "github.com/tendermint/tendermint/libs/net"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/rpc/client/local"
	"google.golang.org/grpc"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"
)

func RandPort() int {
	port, err := tmnet.GetFreePort()
	if err != nil {
		panic(err)
	}
	return port
}

func makeClientAndServer(t *testing.T, routeraddress string) (proto.ApiServiceClient, *RouterConfig) {

	r := NewRouter(routeraddress)

	if r == nil {
		t.Fatal("Failed to create router")
	}
	conn, err := grpc.Dial(routeraddress, grpc.WithInsecure(), grpc.WithContextDialer(dialerFunc))
	if err != nil {
		t.Fatalf("Error Openning GRPC client in router")
	}

	client := proto.NewApiServiceClient(conn)
	return client, r
}
func boostrapBVC(t *testing.T, configfile string, workingdir string, baseport int) error {

	ABCIAddress := fmt.Sprintf("tcp://localhost:%d", baseport)
	RPCAddress := fmt.Sprintf("tcp://localhost:%d", baseport+1)
	GRPCAddress := fmt.Sprintf("tcp://localhost:%d", baseport+2)

	AccRPCInternalAddress := fmt.Sprintf("tcp://localhost:%d", baseport+3) //no longer needed
	RouterPublicAddress := fmt.Sprintf("tcp://localhost:%d", baseport+4)

	//create the default configuration files for the blockchain.
	tendermint.Initialize("accumulate.routertest", ABCIAddress, RPCAddress, GRPCAddress,
		AccRPCInternalAddress, RouterPublicAddress, configfile, workingdir)

	viper.SetConfigFile(configfile)
	viper.AddConfigPath(workingdir)
	viper.ReadInConfig()
	//[mempool]
	//	broadcast = true
	//	cache_size = 100000
	//	max_batch_bytes = 10485760
	//	max_tx_bytes = 1048576
	//	max_txs_bytes = 1073741824
	//	recheck = true
	//	size = 50000
	//	wal_dir = ""
	//
	viper.Set("mempool.cache_size", "1000000")
	viper.Set("mempool.size", "10000")
	viper.WriteConfig()

	return nil
}

func makeBVC(t *testing.T, configfile string, workingdir string) *tendermint.AccumulatorVMApplication {
	app := accnode.CreateAccumulateBVC(configfile, workingdir)
	return app
}

func makeBVCandRouter(t *testing.T, cfg string, dir string) (proto.ApiServiceClient, *RouterConfig, *local.Local, *rpchttp.HTTP) {

	//Select a base port to open.  Ports 43210, 43211, 43212, 43213,43214 need to be open
	baseport := 43210

	//generate the config files needed to run a test BVC
	err := boostrapBVC(t, cfg, dir, baseport)
	if err != nil {
		t.Fatal(err)
	}

	//First we need to build a Router.  The router has to be done first since the BVC connects to it.
	//Make the router's client (i.e. Public facing GRPC client that will route the message to the correct network) and
	//server (i.e. The GRPC that will convert public GRPC messages into messages to communicate with the BVC application)
	viper.SetConfigFile(cfg)
	viper.AddConfigPath(dir)
	viper.ReadInConfig()
	routeraddress := viper.GetString("accumulate.RouterAddress")
	client, routerserver := makeClientAndServer(t, routeraddress)

	///Build a BVC we'll use for our test
	accvm := makeBVC(t, cfg, dir)

	//This will register the Tendermint RPC client of the BVC with the router
	accvmapi, _ := accvm.GetAPIClient()

	//this needs to go away and go through a discovery process instead
	routerserver.AddBVCClient(accvm.GetName(), accvmapi)

	lc, _ := accvm.GetLocalClient()
	laddr := viper.GetString("rpc.laddr")
	//rpcc := tendermint.GetRPCClient(laddr)

	rpcc, _ := rpchttp.New(laddr, "/websocket")

	return client, routerserver, &lc, rpcc
}

func AssembleBVCSubmissionHeader(identityname string, chainname string, ins proto.AccInstruction) *proto.Submission {
	sub := proto.Submission{}

	sub.Identitychain = types.GetIdentityChainFromAdi(identityname).Bytes()
	sub.Chainid = types.GetIdentityChainFromAdi(identityname).Bytes()
	sub.Type = 0 //this is going away it is not needed since we'll know the type from transaction
	sub.Instruction = ins
	return &sub
}

func MakeCreateIdentityBVCSubmission(identityname string, chainname string, payload []byte) *proto.Submission {
	sub := AssembleBVCSubmissionHeader(identityname, chainname, proto.AccInstruction_Identity_Creation)
	sub.Data = payload
	return sub
}

func MakeSignedCreateIdentityBVCSubmission(identityname string, chainname string, payload []byte, signature []byte, pubkey ed25519.PubKey) *proto.Submission {
	sub := MakeCreateIdentityBVCSubmission(identityname, chainname, payload)
	sub.Data = payload
	sub.Signature = signature
	sub.Key = pubkey.Bytes()
	return sub
}

func MakeTokenTransactionBVCSubmission(identityname string, chainname string, payload []byte) *proto.Submission {
	sub := AssembleBVCSubmissionHeader(identityname, chainname, proto.AccInstruction_Token_Transaction)
	sub.Data = payload
	return sub
}

func MakeSignedTokenTransactionBVCSubmission(identityname string, chainname string, payload []byte, signature []byte, pubkey ed25519.PubKey) *proto.Submission {
	sub := MakeTokenTransactionBVCSubmission(identityname, chainname, payload)
	sub.Data = payload
	sub.Signature = signature
	sub.Key = pubkey.Bytes()
	return sub
}

func MakeTokenIssueBVCSubmission(identityname string, chainname string, payload []byte) *proto.Submission {
	sub := AssembleBVCSubmissionHeader(identityname, chainname, proto.AccInstruction_Token_Issue)
	sub.Data = payload
	return sub
}

func MakeSignedTokenIssueBVCSubmission(identityname string, chainname string, payload []byte, signature []byte, pubkey ed25519.PubKey) *proto.Submission {
	sub := MakeTokenIssueBVCSubmission(identityname, chainname, payload)
	sub.Data = payload
	sub.Signature = signature
	sub.Key = pubkey.Bytes()
	return sub
}

func MakeDataChainCreateBVCSubmission(identityname string, chainname string, payload []byte) *proto.Submission {
	sub := AssembleBVCSubmissionHeader(identityname, chainname, proto.AccInstruction_Data_Chain_Creation)
	sub.Data = payload
	return sub
}

func MakeSignedDataChainCreateBVCSubmission(identityname string, chainname string, payload []byte, signature []byte, pubkey ed25519.PubKey) *proto.Submission {
	sub := MakeDataChainCreateBVCSubmission(identityname, chainname, payload)
	sub.Data = payload
	sub.Signature = signature
	sub.Key = pubkey.Bytes()
	return sub
}

func MakeDataEntryBVCSubmission(identityname string, chainname string, payload []byte) *proto.Submission {
	sub := AssembleBVCSubmissionHeader(identityname, chainname, proto.AccInstruction_Data_Entry)
	sub.Data = payload
	return sub
}

func MakeSignedDataEntryBVCSubmission(identityname string, chainname string, payload []byte, signature []byte, pubkey ed25519.PubKey) *proto.Submission {
	sub := MakeDataEntryBVCSubmission(identityname, chainname, payload)
	sub.Data = payload
	sub.Signature = signature
	sub.Key = pubkey.Bytes()
	return sub
}

func MakeDeepQueryBVCSubmission(identityname string, chainname string, payload []byte) *proto.Submission {
	sub := AssembleBVCSubmissionHeader(identityname, chainname, proto.AccInstruction_Deep_Query)
	sub.Data = payload
	return sub
}

func MakeSignedDeepQueryBVCSubmission(identityname string, chainname string, payload []byte, signature []byte, pubkey ed25519.PubKey) *proto.Submission {
	sub := MakeDeepQueryBVCSubmission(identityname, chainname, payload)
	sub.Data = payload
	sub.Signature = signature
	sub.Key = pubkey.Bytes()
	return sub
}

func MakeKeyUpdateBVCSubmission(identityname string, chainname string, payload []byte) *proto.Submission {
	sub := AssembleBVCSubmissionHeader(identityname, chainname, proto.AccInstruction_Key_Update)
	sub.Data = payload
	return sub
}
func MakeSignedKeyUpdateBVCSubmission(identityname string, chainname string, payload []byte, signature []byte, pubkey ed25519.PubKey) *proto.Submission {
	sub := MakeKeyUpdateBVCSubmission(identityname, chainname, payload)
	sub.Signature = signature
	sub.Key = pubkey.Bytes()
	return sub
}

func MakeLightQueryBVCSubmission(identityname string, chainname string, payload []byte) *proto.Submission {
	sub := AssembleBVCSubmissionHeader(identityname, chainname, proto.AccInstruction_Light_Query)
	sub.Data = payload
	return sub
}

var UrlInstructionMap = map[string]proto.AccInstruction{
	"identity-create":      proto.AccInstruction_Identity_Creation,
	"idc":                  proto.AccInstruction_Identity_Creation,
	"token-url-create":     proto.AccInstruction_Token_URL_Creation,
	"url":                  proto.AccInstruction_Token_URL_Creation,
	"token-tx":             proto.AccInstruction_Token_Transaction,
	"tx":                   proto.AccInstruction_Token_Transaction,
	"data-chain-create":    proto.AccInstruction_Data_Chain_Creation,
	"dcc":                  proto.AccInstruction_Data_Chain_Creation,
	"data-entry":           proto.AccInstruction_Data_Entry,
	"de":                   proto.AccInstruction_Data_Entry,
	"scratch-chain-create": proto.AccInstruction_Scratch_Chain_Creation,
	"scc":                  proto.AccInstruction_Scratch_Chain_Creation,
	"scratch-entry":        proto.AccInstruction_Scratch_Entry,
	"se":                   proto.AccInstruction_Scratch_Entry,
	"token-issue":          proto.AccInstruction_Token_Issue,
	"ti":                   proto.AccInstruction_Token_Issue,
	"key-update":           proto.AccInstruction_Key_Update,
	"ku":                   proto.AccInstruction_Key_Update,
	"deep-query":           proto.AccInstruction_Deep_Query,
	"dq":                   proto.AccInstruction_Deep_Query,
	"query":                proto.AccInstruction_Light_Query,
	"q":                    proto.AccInstruction_Light_Query,
}

func URLParser(s string) (ret *proto.Submission, err error) {

	if !utf8.ValidString(s) {
		return ret, fmt.Errorf("URL is has invalid UTF8 encoding")
	}

	if !strings.HasPrefix(s, "acc://") {
		s = "acc://" + s
	}

	var sub *proto.Submission

	u, err := url.Parse(s)
	if err != nil {
		return ret, err
	}

	fmt.Println(u.Scheme)

	fmt.Println(u.Host)
	//so the primary is up to the "." if it is there.
	hostname := u.Hostname()
	//DDIIaccounts := strings.Split(hostname,".")
	m, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return ret, err
	}

	//todo .. make the correct submission based upon raw query...
	for k, _ := range m {
		m.Del(k)
		js, _ := toJSON(m)
		switch k {
		case "q":
			fallthrough
		case "query":
			sub = MakeLightQueryBVCSubmission(hostname, hostname+u.Path, []byte(js))
			fmt.Println(js)
		case "tx":
			fallthrough
		case "token-tx":
			data, err := hex.DecodeString(m["payload"])
			signature, err := hex.DecodeString(m["sig"])
			key, err := hex.DecodeString(m["key"])
			if err != nil {
				return nil, fmt.Errorf("Cannot decode signature in url %v", err)
			}
			sub = MakeSignedTokenTransactionBVCSubmission(hostname, hostname+u.Path, data, signature, key)
			fmt.Println(js)
		}
		break
	}
	if sub == nil {
		sub = &proto.Submission{
			Identitychain: types.GetIdentityChainFromAdi(hostname).Bytes(),
			Chainid:       types.GetIdentityChainFromAdi(hostname + u.Path).Bytes(),
			Instruction:   proto.AccInstruction_Unknown,
		}
	}
	//}

	return &sub, nil
}
