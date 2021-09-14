package router

import (
	"context"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	types2 "github.com/tendermint/tendermint/abci/types"

	proto1 "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	tmnet "github.com/tendermint/tendermint/libs/net"
	coregrpc "github.com/tendermint/tendermint/rpc/grpc"
	"google.golang.org/grpc"

	//vtypes "github.com/AccumulateNetwork/accumulated/blockchain/validator/types"
	"net"
	"net/url"
)

type RouterConfig struct {
	proto.ApiServiceServer
	grpcServer *grpc.Server

	bvcclients []coregrpc.BroadcastAPIClient
	Address    string
}

func (app *RouterConfig) PostEntry(context.Context, *proto.EntryBytes) (*proto.Reply, error) {
	return nil, nil
}
func (app *RouterConfig) ReadKeyValue(context.Context, *proto.Key) (*proto.KeyValue, error) {
	return nil, nil
}
func (app *RouterConfig) RequestAccount(context.Context, *proto.Key) (*proto.Account, error) {
	return nil, nil
}
func (app *RouterConfig) GetHeight(context.Context, *empty.Empty) (*proto.Height, error) {
	return nil, nil
}
func (app *RouterConfig) GetNodeInfo(context.Context, *empty.Empty) (*proto.NodeInfo, error) {
	return nil, nil
}

func (app *RouterConfig) QueryShardCount(context.Context, *empty.Empty) (*proto.ShardCountResponse, error) {
	scr := proto.ShardCountResponse{}

	// fmt.Printf("TODO: need to implement blockchain query to dbvc for number of shards\n")
	scr.Numshards = app.GetNumShardsInSystem() //todo: Need to query blockchain for this number....
	return &scr, nil
}

func (app *RouterConfig) GetNumShardsInSystem() int32 {
	//todo query DBVC....  need a better way to monitor this from the perspective of the DBVC since it knows about
	//everyone.  We also need to know who all the leaders are who participated in a given round at a given height
	//only update it once in a blue moon when shards are added.  so need to monitor the appropriate chain for activity.
	return 1
}

// ProcessQuery processes a query
func (app *RouterConfig) ProcessQuery(ctx context.Context, query *proto.Query) (*proto.QueryResp, error) {
	resp := proto.QueryResp{}
	client := app.getBVCClient(types.GetAddressFromIdentityChain(query.AdiChain))
	if client == nil {
		resp.Data = nil
		resp.Code = 0x0001
		return nil, fmt.Errorf("no BVC's defined for router")
	}

	var err error
	rq := types2.RequestQuery{}
	rq.Data, err = proto1.Marshal(query)

	if err != nil {
		return nil, err
	}

	//resp, err = client.QuerySync(rq)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func (app *RouterConfig) ProcessTx(ctx context.Context, sub *proto.Submission) (*proto.SubmissionResponse, error) {
	resp := proto.SubmissionResponse{}
	client := app.getBVCClient(types.GetAddressFromIdentityChain(sub.Identitychain))
	if client == nil {
		resp.Respdata = nil
		resp.ErrorCode = 0x0001
		return nil, fmt.Errorf("no BVC's defined for router")
	}

	msg, err := proto1.Marshal(sub)
	if err != nil {
		return nil, fmt.Errorf("invalid Submission payload for Synthetic TX")
	}
	req := coregrpc.RequestBroadcastTx{}
	req.Tx = msg
	//probably should shift to https submissions.
	client.BroadcastTx(context.Background(), &req)

	resp.Respdata = nil
	resp.ErrorCode = 0x9000
	return &resp, nil
}

func (app *RouterConfig) Close() {
	app.grpcServer.GracefulStop()
}

func NewRouter(routeraddress string) (config *RouterConfig) {
	r := RouterConfig{}

	r.Address = routeraddress

	if len(r.Address) == 0 {
		panic("accumulate.RouterAddress token not specified in config file")
	}
	urladdr, err := url.Parse(r.Address)
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

func (app *RouterConfig) getBVCClient(addr uint64) coregrpc.BroadcastAPIClient {
	numbvcnetworks := uint64(len(app.bvcclients))
	if numbvcnetworks == 0 {
		return nil
	}
	return app.bvcclients[addr%numbvcnetworks]
}

func (app *RouterConfig) AddBVCClient(shardname string, client coregrpc.BroadcastAPIClient) error {
	//todo: make this a discovery method.  we need to know for sure how many BVC's there are and we need
	//to explicitly connect to them...
	app.bvcclients = append(app.bvcclients, client)
	return nil
}

func dialerFunc(ctx context.Context, addr string) (net.Conn, error) {
	return tmnet.Connect(addr)
}

func (app *RouterConfig) CreateGRPCClient() (proto.ApiServiceClient, error) {
	conn, err := grpc.Dial(app.Address, grpc.WithInsecure(), grpc.WithContextDialer(dialerFunc))
	if err != nil {
		return nil, fmt.Errorf("Error Openning GRPC client in router")
	}
	api := proto.NewApiServiceClient(conn)
	return api, nil
}
