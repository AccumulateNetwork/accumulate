package router

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/AccumulateNetwork/SMT/smt"
	"github.com/AccumulateNetwork/accumulated/proto"
	proto1 "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	abcicli "github.com/tendermint/tendermint/abci/client"
	"github.com/tendermint/tendermint/abci/types"
	tmnet "github.com/tendermint/tendermint/libs/net"
	"google.golang.org/grpc"
	"net"
	"net/url"
	"strings"
)

type RouterConfig struct {
	proto.ApiServiceServer
	shardclients []abcicli.Client
	Address string

}
func (app RouterConfig) PostEntry(context.Context, *proto.EntryBytes) (*proto.Reply, error) {
	return nil,nil
}
func (app RouterConfig) ReadKeyValue(context.Context, *proto.Key) (*proto.KeyValue, error) {
	return nil,nil
}
func (app RouterConfig) RequestAccount(context.Context, *proto.Key) (*proto.Account, error) {
	return nil,nil
}
func (app RouterConfig) GetHeight(context.Context, *empty.Empty) (*proto.Height, error) {
	return nil,nil
}
func (app RouterConfig) GetNodeInfo(context.Context, *empty.Empty) (*proto.NodeInfo, error) {
	return nil,nil
}


func (app RouterConfig) SyntheticTx(ctx context.Context,sub *proto.Submission) (*proto.SubmissionResponse, error) {
	fmt.Printf("hello world from dispatch server TX ")
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
	grpcServer := grpc.NewServer(opts...)
	proto.RegisterApiServiceServer(grpcServer, &r)
	go grpcServer.Serve(lis)

	return &r
}

func (app RouterConfig) getBVCClient(addr uint64) abcicli.Client {
	numshards := uint64(len(app.shardclients))
	if numshards == 0 {
		return nil
	}
	return app.shardclients[addr%numshards]
}

func (app RouterConfig) AddShardClient(shardname string, client abcicli.Client) error {
	app.shardclients = append(app.shardclients,client)
	return nil
}

func dialerFunc(ctx context.Context, addr string) (net.Conn, error) {
	return tmnet.Connect(addr)
}

func (app RouterConfig) CreateGRPCClient() (proto.ApiServiceClient,error) {
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

