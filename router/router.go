package router

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/SMT/smt"
	"github.com/AccumulateNetwork/accumulated/proto"
	"github.com/AccumulateNetwork/accumulated/validator"
	proto1 "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	abcicli "github.com/tendermint/tendermint/abci/client"
	"github.com/tendermint/tendermint/abci/types"
	tmnet "github.com/tendermint/tendermint/libs/net"
	"google.golang.org/grpc"
	"net"
	"net/url"
	"strings"
	"unicode/utf8"
)

type RouterConfig struct {
	proto.ApiServiceServer
	grpcServer *grpc.Server

	bvcclients []abcicli.Client
	Address string
}


func (app *RouterConfig) PostEntry(context.Context, *proto.EntryBytes) (*proto.Reply, error) {
	return nil,nil
}
func (app *RouterConfig) ReadKeyValue(context.Context, *proto.Key) (*proto.KeyValue, error) {
	return nil,nil
}
func (app *RouterConfig) RequestAccount(context.Context, *proto.Key) (*proto.Account, error) {
	return nil,nil
}
func (app *RouterConfig) GetHeight(context.Context, *empty.Empty) (*proto.Height, error) {
	return nil,nil
}
func (app *RouterConfig) GetNodeInfo(context.Context, *empty.Empty) (*proto.NodeInfo, error) {
	return nil,nil
}

func (app *RouterConfig) QueryShardCount(context.Context, *empty.Empty) (*proto.ShardCountResponse, error) {
	scr := proto.ShardCountResponse{}

	fmt.Printf("TODO: need to implement blockchain query to dbvc for number of shards\n")
	scr.Numshards = app.GetNumShardsInSystem() //todo: Need to query blockchain for this number....
	return &scr,nil
}

func (app *RouterConfig) GetNumShardsInSystem() int32 {
	//todo query DBVC....  need a better way to monitor this from the perspective of the DBVC since it knows about
	//everyone.  We also need to know who all the leaders are who participated in a given round at a given height
	//only update it once in a blue moon when shards are added.  so need to monitor the appropriate chain for activity.
	return 1
}

func (app *RouterConfig) Query(ctx context.Context,query *proto.AccQuery) (*proto.AccQueryResp, error) {
	scr := proto.AccQueryResp{}

	rq := types.RequestQuery{}

	rq.Data,_ = proto1.Marshal(query)

	rq.Height = 12345
	client := app.getBVCClient(query.Addr)
	if client == nil {
		return nil, fmt.Errorf("No BVC Client Available on for address %X", query.Addr)
	}
	resp, err := client.QuerySync(rq)

	if err != nil {
		return nil, err
	}

	scr.Code = resp.Code

	return &scr,nil
}

func (app *RouterConfig) ProcessTx(ctx context.Context,sub *proto.Submission) (*proto.SubmissionResponse, error) {
	//fmt.Printf("hello world from dispatch server TX ")
	resp := proto.SubmissionResponse{}
	client := app.getBVCClient(sub.GetAddress())
	if client == nil {
		resp.Respdata = nil
		resp.ErrorCode = 0x0001
		return nil, fmt.Errorf("No BVC's defined for router")
	}

	msg, err := proto1.Marshal(sub)
	if err != nil {
		return nil, fmt.Errorf("Invalid Submission payload for Synthetic TX")
	}
	req := types.RequestCheckTx{}
	req.Tx = msg
	checkresp,err := client.CheckTxSync(req)
	if err != nil {
		fmt.Printf("Error received from Check Tx Sync %v", err)
	}
	fmt.Printf("Result : %s", checkresp.Log)

	resp.Respdata = nil
	resp.ErrorCode = 0x9000
	return &resp, nil
}

func (app *RouterConfig) Close (){
    app.grpcServer.GracefulStop()
}

func NewRouter(routeraddress string) (config *RouterConfig) {
	r := RouterConfig{}

	r.Address = routeraddress

	if len(r.Address) == 0 {
		panic("accumulate.RouterAddress token not specified in config file")
	}
	urladdr,err := url.Parse(r.Address)
	if err != nil {
		panic(err)
	}


	lis, err := net.Listen(urladdr.Scheme, urladdr.Host)


	if err != nil {
		panic(fmt.Sprintf("failed to listen: %v", err))
	}
	var opts []grpc.ServerOption
	//...
	r.grpcServer = grpc.NewServer(opts...)
	proto.RegisterApiServiceServer(r.grpcServer, &r)
	go r.grpcServer.Serve(lis)

	return &r
}

func (app *RouterConfig) getBVCClient(addr uint64) abcicli.Client {
	numbvcnetworks := uint64(len(app.bvcclients))
	if numbvcnetworks == 0 {
		return nil
	}
	return app.bvcclients[addr%numbvcnetworks]
}

func (app *RouterConfig) AddBVCClient(shardname string, client abcicli.Client) error {
	//todo: make this a discovery method.  we need to know for sure how many BVC's there are and we need
	//to explicitly connect to them...
	app.bvcclients = append(app.bvcclients,client)
	return nil
}

func dialerFunc(ctx context.Context, addr string) (net.Conn, error) {
	return tmnet.Connect(addr)
}

func (app *RouterConfig) CreateGRPCClient() (proto.ApiServiceClient,error) {
	conn, err := grpc.Dial(app.Address, grpc.WithInsecure(), grpc.WithContextDialer(dialerFunc))
	if err != nil {
		return nil, fmt.Errorf("Error Openning GRPC client in router")
	}
	api := proto.NewApiServiceClient(conn)
	return api, nil
}


func GetAddressFromIdentityName(name string) uint64 {
	namelower := strings.ToLower(name)
	h := sha256.Sum256([]byte(namelower))

	addr,_ := smt.BytesUint64(h[:])
	return addr
}

func SendTransaction(senderurl string, receiverurl string) error {
	su, err := url.Parse(senderurl)
	if err != nil {
		return fmt.Errorf("Unable to parse Sender URL %v",err)
	}
	ru, err := url.Parse(receiverurl)
	if err != nil {
		return fmt.Errorf("Unable to parse Receiver URL %v",err)
	}

	//convert URL host identity to lowercase standard
	senderidentity := strings.ToLower(su.Host)
	receiveridentity := strings.ToLower(ru.Host)

	sh := sha256.Sum256([]byte(senderidentity))
	rh := sha256.Sum256([]byte(receiveridentity))


	sendaddr,_ := smt.BytesUint64(sh[:])
	recvaddr,_ := smt.BytesUint64(rh[:])

	sendtokentype := su.Path

	receivetokentype := ru.Path

	fmt.Printf("%d%d%s%s", sendaddr, recvaddr, sendtokentype,receivetokentype )

    return nil
}
//
//const (
//	AccAction_Unknown = iota
//	AccAction_Identity_Creation
//	AccAction_Token_URL_Creation
//	AccAction_Token_Transaction
//	AccAction_Data_Chain_Creation
//	AccAction_Data_Entry //per 250 bytes
//	AccAction_Scratch_Chain_Creation
//	AccAction_Scratch_Entry //per 250 bytes
//	AccAction_Token_Issue
//	AccAction_Key_Update
//)

type AccUrl struct {
	Addr uint64
	DDII string
	ChainPath []smt.Hash
	Action uint32
}
//func (app AccUrl) Marshall() ([]byte,error) {
//	m := make([]byte, 8 + len(app.DDII)+ len(app.ChainPath)*32 + 4 )
//	copy(m)
//	return m, nil
//}
func toJSON(m interface{}) (string,error) {
	js, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(js), ",", ", "),nil
}

func URLParser(s string) (ret proto.AccQuery,err error) {

	if !utf8.ValidString(s) {
		return ret,fmt.Errorf("URL is has invalid UTF8 encoding")
	}

	if !strings.HasPrefix(s,"acc://") {
		s = "acc://" + s
	}


	u, err := url.Parse(s)
	if err != nil {
		return ret, err
	}

	fmt.Println(u.Scheme)

	fmt.Println(u.Host)
	//so the primary is up to the "." if it is there.
	hostname := u.Hostname()
	//DDIIaccounts := strings.Split(hostname,".")
	ret.DDII = hostname

	ret.Addr = validator.GetTypeIdFromName(hostname)

	m, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return ret,err
	}
	js,err := toJSON(m)

	fmt.Println(js)

	h := sha256.Sum256([]byte(hostname+u.Path))
	ret.ChainId =  h[:]
    ret.Query = js //u.RawQuery

    return ret,nil
}

func Router() {
//acc://root_name[/sub-chain name[/sub-chain]...]
//acc://a:Big.Company/atk
	//s := "postgres://user:pass@host.com:5432/path?k=v#f"

	s := "acc://Big.Company/fct?send-transaction/Small.Company/fct"
    //s = url.QueryEscape(s)

	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}

	fmt.Println(u.Scheme)

	fmt.Println(u.User)
	fmt.Println(u.User.Username())
	p, _ := u.User.Password()
	fmt.Println(p)

	fmt.Println(u.Host)
	host, port, _ := net.SplitHostPort(u.Host)
	fmt.Println(host)
	fmt.Println(port)

	fmt.Println(u.Path)
	fmt.Println(u.Fragment)

	fmt.Println(u.RawQuery)
	m, _ := url.ParseQuery(u.RawQuery)
	fmt.Println(m)
	//fmt.Println(m["k"][0])
}

