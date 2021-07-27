package router

import (
	"crypto/sha256"
	"encoding/binary"
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
	"strconv"
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

func AssembleBVCSubmissionHeader(identityname string, chainpath string, ins proto.AccInstruction) *proto.Submission {
	sub := proto.Submission{}

	sub.Identitychain = types.GetIdentityChainFromAdi(identityname).Bytes()
	if chainpath == "" {
		chainpath = identityname
	}
	sub.Chainid = types.GetIdentityChainFromAdi(chainpath).Bytes()
	sub.Type = 0 //this is going away it is not needed since we'll know the type from transaction
	sub.Instruction = ins
	return &sub
}

func MakeBVCSubmission(ins string, identityname string, chainpath string, payload []byte, timestamp int64, signature []byte, pubkey ed25519.PubKey) *proto.Submission {
	v := InstructionTypeMap[ins]
	if v == 0 {
		return nil
	}
	sub := AssembleBVCSubmissionHeader(identityname, chainpath, v)
	sub.Data = payload
	sub.Timestamp = timestamp
	sub.Signature = signature
	sub.Key = pubkey
	return sub
}

//fullchainpath == identityname/chainpath
func MarshalBinarySig(fullchainpath string, payload []byte, timestamp int64) []byte {
	var msg []byte

	//The chain path is either the identity name or the full chain path [identityname]/[chainpath]
	chainid := sha256.Sum256([]byte(fullchainpath))
	msg = append(msg, chainid[:]...)

	payloadhash := sha256.Sum256(payload)
	msg = append(msg, payloadhash[:]...)

	var tsbytes [8]byte
	binary.LittleEndian.PutUint64(tsbytes[:], uint64(timestamp))
	msg = append(msg, tsbytes[:]...)

	return msg
}

var InstructionTypeMap = map[string]proto.AccInstruction{
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
	hostname := strings.ToLower(u.Hostname())
	//DDIIaccounts := strings.Split(hostname,".")

	m, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return ret, err
	}

	chainpath := hostname
	if len(u.Path) != 0 {
		chainpath += strings.ToLower(u.Path)
	}

	insidx := strings.Index(u.RawQuery, "&")
	if len(u.RawQuery) > 0 && insidx < 0 {
		insidx = len(u.RawQuery)
	}

	if insidx > 0 {
		k := u.RawQuery[:insidx]
		var data []byte
		if k == "query" || k == "q" {
			if v := m["payload"]; v == nil {
				m.Del(k)
				js, err := toJSON(m)
				if err != nil {
					return nil, fmt.Errorf("Unable to create url query %s, %v", s, err)
				}
				data = []byte(js)

			}
		}
		//make the correct submission based upon raw query...  Light query needs to be handled differently.
		var timestamp int64
		var signature []byte
		var key []byte
		if v := m["payload"]; v != nil {
			if len(v) > 0 {
				data, err = hex.DecodeString(m["payload"][0])
				if err != nil {
					return nil, fmt.Errorf("Unable to parse payload in url %s, %v", s, err)
				}
			}
		}
		if v := m["timestamp"]; v != nil {
			if len(v) > 0 {
				timestamp, err = strconv.ParseInt(v[0], 10, 64)
				if err != nil {
					return nil, fmt.Errorf("Unable to parse timestamp in url %s, %v", s, err)
				}
			}
		}

		if v := m["sig"]; v != nil {
			if len(v) > 0 {
				signature, err = hex.DecodeString(m["sig"][0])
				if err != nil {
					return nil, fmt.Errorf("Unable to parse signature in url %s, %v", s, err)
				}
			}
		}
		if v := m["key"]; v != nil {
			if len(v) > 0 {
				key, err = hex.DecodeString(m["key"][0])
				if err != nil {
					return nil, fmt.Errorf("Unable to parse signature in url %s, %v", s, err)
				}
			}
		}

		sub = MakeBVCSubmission(k, hostname, chainpath, data, timestamp, signature, key)
	}

	if sub == nil {
		sub = AssembleBVCSubmissionHeader(hostname, chainpath, proto.AccInstruction_Unknown)
	}

	return sub, nil
}
