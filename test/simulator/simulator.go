// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	btc "github.com/btcsuite/btcd/btcec"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
	"golang.org/x/sync/errgroup"
)

type Simulator struct {
	logger     logging.OptionalLogger
	init       *accumulated.NetworkInit
	database   OpenDatabaseFunc
	partitions map[string]*Partition
	router     *Router
	netcfg     *config.Network

	blockErrGroup *errgroup.Group
}

type OpenDatabaseFunc func(partition string, node int, logger log.Logger) database.Beginner
type SnapshotFunc func(partition string, network *accumulated.NetworkInit, logger log.Logger) (ioutil2.SectionReader, error)

func New(logger log.Logger, database OpenDatabaseFunc, network *accumulated.NetworkInit, snapshot SnapshotFunc) (*Simulator, error) {
	s := new(Simulator)
	s.logger.Set(logger, "module", "sim")
	s.init = network
	s.database = database
	s.partitions = make(map[string]*Partition, len(network.Bvns)+1)
	s.router = newRouter(logger, s.partitions)

	s.netcfg = new(config.Network)
	s.netcfg.Id = network.Id
	s.netcfg.Partitions = make([]config.Partition, len(network.Bvns)+1)
	s.netcfg.Partitions[0].Id = protocol.Directory
	s.netcfg.Partitions[0].Type = config.Directory
	for i, bvn := range network.Bvns {
		s.netcfg.Partitions[i+1].Id = bvn.Id
		s.netcfg.Partitions[i+1].Type = config.BlockValidator
		s.netcfg.Partitions[i+1].Nodes = make([]config.Node, len(bvn.Nodes))
		for j, node := range bvn.Nodes {
			s.netcfg.Partitions[i+1].Nodes[j].Address = node.AdvertizeAddress
			s.netcfg.Partitions[i+1].Nodes[j].Type = node.BvnnType

			dnn := config.Node{Address: node.AdvertizeAddress, Type: node.DnnType}
			s.netcfg.Partitions[0].Nodes = append(s.netcfg.Partitions[0].Nodes, dnn)

			if node.BasePort != 0 {
				s.netcfg.Partitions[i+1].Nodes[j].Address = node.Advertize().Scheme("http").BlockValidator().String()
				s.netcfg.Partitions[0].Nodes[j].Address = node.Advertize().Scheme("http").Directory().String()
			}
		}
	}

	var err error
	s.partitions[protocol.Directory], err = newDn(s, network)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	for _, bvn := range network.Bvns {
		s.partitions[bvn.Id], err = newBvn(s, bvn)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	for _, p := range s.partitions {
		snapshot, err := snapshot(p.ID, s.init, s.logger)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("open snapshot: %w", err)
		}
		err = p.initChain(snapshot)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("init %s: %w", p.ID, err)
		}
	}
	return s, nil
}

func MemoryDatabase(_ string, _ int, logger log.Logger) database.Beginner {
	return database.OpenInMemory(logger)
}

func BadgerDatabaseFromDirectory(dir string, onErr func(error)) OpenDatabaseFunc {
	return func(partition string, node int, logger log.Logger) database.Beginner {
		err := os.MkdirAll(dir, 0700)
		if err != nil {
			onErr(err)
			panic(err)
		}

		db, err := database.OpenBadger(filepath.Join(dir, fmt.Sprintf("%s-%d.db", partition, node)), logger)
		if err != nil {
			onErr(err)
			panic(err)
		}

		return db
	}
}

// SimpleNetwork creates a basic network with the given name, number of BVNs,
// and number of nodes per BVN.
func SimpleNetwork(name string, bvnCount, nodeCount int) *accumulated.NetworkInit {
	net := new(accumulated.NetworkInit)
	net.Id = name
	for i := 0; i < bvnCount; i++ {
		bvnInit := new(accumulated.BvnInit)
		bvnInit.Id = fmt.Sprintf("BVN%d", i)
		for j := 0; j < nodeCount; j++ {
			bvnInit.Nodes = append(bvnInit.Nodes, &accumulated.NodeInit{
				DnnType:    config.Validator,
				BvnnType:   config.Validator,
				PrivValKey: testing.GenerateKey(name, bvnInit.Id, j, "val"),
				DnNodeKey:  testing.GenerateKey(name, bvnInit.Id, j, "dn"),
				BvnNodeKey: testing.GenerateKey(name, bvnInit.Id, j, "bvn"),
			})
		}
		net.Bvns = append(net.Bvns, bvnInit)
	}
	return net
}

// LocalNetwork returns a SimpleNetwork with sequential IPs starting from the
// base IP with the given base port.
func LocalNetwork(name string, bvnCount, nodeCount int, baseIP net.IP, basePort uint64) *accumulated.NetworkInit {
	net := SimpleNetwork(name, bvnCount, nodeCount)
	for _, bvn := range net.Bvns {
		for _, node := range bvn.Nodes {
			node.AdvertizeAddress = baseIP.String()
			node.BasePort = basePort
			baseIP[len(baseIP)-1]++
		}
	}
	return net
}

func (s *Simulator) SetRoute(account *url.URL, partition string) {
	s.router.SetRoute(account, partition)
}

func SnapshotFromDirectory(dir string) SnapshotFunc {
	return func(partition string, network *accumulated.NetworkInit, logger log.Logger) (ioutil2.SectionReader, error) {
		return os.Open(filepath.Join(dir, fmt.Sprintf("%s.snapshot", partition)))
	}
}

func SnapshotMap(snapshots map[string]ioutil2.SectionReader) SnapshotFunc {
	return func(partition string, _ *accumulated.NetworkInit, _ log.Logger) (ioutil2.SectionReader, error) {
		return snapshots[partition], nil
	}
}

func Genesis(time time.Time) SnapshotFunc {
	return GenesisWith(time, nil)
}

func GenesisWith(time time.Time, values *core.GlobalValues) SnapshotFunc {
	if values == nil {
		values = new(core.GlobalValues)
	}

	var genDocs map[string]*tmtypes.GenesisDoc
	return func(partition string, network *accumulated.NetworkInit, logger log.Logger) (ioutil2.SectionReader, error) {
		var err error
		if genDocs == nil {
			genDocs, err = accumulated.BuildGenesisDocs(network, values, time, logger, nil, nil)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("build genesis docs: %w", err)
			}
		}

		var snapshot []byte
		err = json.Unmarshal(genDocs[partition].AppState, &snapshot)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

		return ioutil2.NewBuffer(snapshot), nil
	}
}

func (s *Simulator) Router() routing.Router { return s.router }
func (s *Simulator) EventBus() *events.Bus  { return s.partitions[protocol.Directory].nodes[0].eventBus }

// Step executes a single simulator step
func (s *Simulator) Step() error {
	s.blockErrGroup = new(errgroup.Group)
	for _, p := range s.partitions {
		p := p // Don't capture loop variables
		s.blockErrGroup.Go(p.execute)
	}
	return s.blockErrGroup.Wait()
}

func (s *Simulator) SetSubmitHookFor(account *url.URL, fn SubmitHookFunc) {
	partition, err := s.router.RouteAccount(account)
	if err != nil {
		panic(err)
	}
	s.partitions[partition].SetSubmitHook(fn)
}

func (s *Simulator) SetSubmitHook(partition string, fn SubmitHookFunc) {
	s.partitions[partition].SetSubmitHook(fn)
}

func (s *Simulator) SetRouterSubmitHookFor(account *url.URL, fn RouterSubmitHookFunc) {
	partition, err := s.router.RouteAccount(account)
	if err != nil {
		panic(err)
	}
	s.partitions[partition].SetRouterSubmitHook(fn)
}

func (s *Simulator) SetRouterSubmitHook(partition string, fn RouterSubmitHookFunc) {
	s.partitions[partition].SetRouterSubmitHook(fn)
}

func (s *Simulator) Submit(delivery *chain.Delivery) (*protocol.TransactionStatus, error) {
	partition, err := s.router.Route(&protocol.Envelope{
		Transaction: []*protocol.Transaction{delivery.Transaction},
		Signatures:  delivery.Signatures,
	})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return s.SubmitTo(partition, delivery)
}

func (s *Simulator) SubmitTo(partition string, delivery *chain.Delivery) (*protocol.TransactionStatus, error) {
	p, ok := s.partitions[partition]
	if !ok {
		return nil, errors.BadRequest.WithFormat("%s is not a partition", partition)
	}

	return p.Submit(&messaging.LegacyMessage{
		Transaction: delivery.Transaction,
		Signatures:  delivery.Signatures,
	}, false)
}

func (s *Simulator) Partitions() []*protocol.PartitionInfo {
	var partitions []*protocol.PartitionInfo
	for _, p := range s.partitions {
		partitions = append(partitions, &p.PartitionInfo)
	}
	return partitions
}

type errDb struct{ err error }

func (e errDb) View(func(*database.Batch) error) error   { return e.err }
func (e errDb) Update(func(*database.Batch) error) error { return e.err }
func (e errDb) Begin(bool) *database.Batch               { panic(e.err) }

func (s *Simulator) Database(partition string) database.Beginner {
	p, ok := s.partitions[partition]
	if !ok {
		return errDb{errors.BadRequest.WithFormat("%s is not a partition", partition)}
	}
	return p
}

func (s *Simulator) DatabaseFor(account *url.URL) database.Updater {
	partition, err := s.router.RouteAccount(account)
	if err != nil {
		return errDb{errors.UnknownError.Wrap(err)}
	}
	return s.Database(partition)
}

func (s *Simulator) ClientV2(part string) *client.Client {
	return s.partitions[part].nodes[0].clientV2
}

func (s *Simulator) ViewAll(fn func(batch *database.Batch) error) error {
	for _, p := range s.partitions {
		err := p.View(fn)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Simulator) NewDirectClient() *client.Client {
	return testing.DirectJrpcClient(s.partitions[protocol.Directory].nodes[0].apiV2)
}

func (s *Simulator) ListenAndServe(ctx context.Context, hook func(*Simulator, http.Handler) http.Handler) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errg := new(errgroup.Group)
	for _, part := range s.partitions {
		for _, node := range part.nodes {
			var addr string
			if part.Type == config.Directory {
				addr = node.init.Listen().Directory().AccumulateAPI().String()
			} else {
				addr = node.init.Listen().BlockValidator().AccumulateAPI().String()
			}

			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}
			defer func() { _ = ln.Close() }()

			srv := http.Server{Handler: node.apiV2.NewMux()}
			if hook != nil {
				srv.Handler = hook(s, srv.Handler)
			}

			go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
			errg.Go(func() error { return srv.Serve(ln) })

			s.logger.Info("Node API up", "partition", part.ID, "node", node.id, "address", "http://"+addr)
		}
	}
	return errg.Wait()
}

func (s *Simulator) SignWithNode(partition string, i int) nodeSigner {
	return nodeSigner{s.partitions[partition].nodes[i]}
}

type nodeSigner struct {
	*Node
}

var _ signing.Signer = nodeSigner{}

func (n nodeSigner) Key() []byte { return n.init.PrivValKey }

func (n nodeSigner) SetPublicKey(sig protocol.Signature) error {
	k := n.init.PrivValKey
	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		sig.PublicKey = k[32:]

	case *protocol.ED25519Signature:
		sig.PublicKey = k[32:]

	case *protocol.RCD1Signature:
		sig.PublicKey = k[32:]

	case *protocol.BTCSignature:
		_, pubKey := btc.PrivKeyFromBytes(btc.S256(), k)
		sig.PublicKey = pubKey.SerializeCompressed()

	case *protocol.BTCLegacySignature:
		_, pubKey := btc.PrivKeyFromBytes(btc.S256(), k)
		sig.PublicKey = pubKey.SerializeUncompressed()

	case *protocol.ETHSignature:
		_, pubKey := btc.PrivKeyFromBytes(btc.S256(), k)
		sig.PublicKey = pubKey.SerializeUncompressed()

	default:
		return fmt.Errorf("cannot set the public key on a %T", sig)
	}

	return nil
}

func (n nodeSigner) Sign(sig protocol.Signature, sigMdHash, message []byte) error {
	k := n.init.PrivValKey
	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		protocol.SignLegacyED25519(sig, k, sigMdHash, message)

	case *protocol.ED25519Signature:
		protocol.SignED25519(sig, k, sigMdHash, message)

	case *protocol.RCD1Signature:
		protocol.SignRCD1(sig, k, sigMdHash, message)

	case *protocol.BTCSignature:
		return protocol.SignBTC(sig, k, sigMdHash, message)

	case *protocol.BTCLegacySignature:
		return protocol.SignBTCLegacy(sig, k, sigMdHash, message)

	case *protocol.ETHSignature:
		return protocol.SignETH(sig, k, sigMdHash, message)

	default:
		return fmt.Errorf("cannot sign %T with a key", sig)
	}
	return nil
}

// Services returns the simulator's API v3 implementation.
func (s *Simulator) Services() *simService { return (*simService)(s) }

// simService implements API v3.
type simService Simulator

// Private returns the private sequencer service.
func (s *simService) Private() private.Sequencer { return s }

// NodeStatus finds the specified node and returns its NodeStatus.
func (s *simService) NodeStatus(ctx context.Context, opts api.NodeStatusOptions) (*api.NodeStatus, error) {
	if opts.NodeID == "" {
		return nil, errors.BadRequest.WithFormat("node ID is missing")
	}
	id, err := peer.Decode(opts.NodeID)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("invalid peer ID: %w", err)
	}
	for _, p := range s.partitions {
		for _, n := range p.nodes {
			sk, err := crypto.UnmarshalEd25519PrivateKey(n.nodeKey)
			if err != nil {
				continue
			}
			if id.MatchesPrivateKey(sk) {
				return (*nodeService)(n).NodeStatus(ctx, opts)
			}
		}
	}
	return nil, errors.NotFound.WithFormat("node %s not found", id)
}

// NetworkStatus implements pkg/api/v3.NetworkService.
func (s *simService) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	return (*nodeService)(s.partitions[protocol.Directory].nodes[0]).NetworkStatus(ctx, opts)
}

// Metrics implements pkg/api/v3.MetricsService.
func (s *simService) Metrics(ctx context.Context, opts api.MetricsOptions) (*api.Metrics, error) {
	return nil, errors.NotAllowed
}

// Query routes the scope to a partition and calls Query on the first node of
// that partition, returning the result.
func (s *simService) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	part, err := s.router.RouteAccount(scope)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return (*nodeService)(s.partitions[part].nodes[0]).Query(ctx, scope, query)
}

// Submit routes the envelope to a partition and calls Submit on the first node
// of that partition, returning the result.
func (s *simService) Submit(ctx context.Context, envelope *protocol.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	part, err := s.router.Route(envelope)
	if err != nil {
		return nil, err
	}
	return (*nodeService)(s.partitions[part].nodes[0]).Submit(ctx, envelope, opts)
}

// Validate routes the envelope to a partition and calls Validate on the first
// node of that partition, returning the result.
func (s *simService) Validate(ctx context.Context, envelope *protocol.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	part, err := s.router.Route(envelope)
	if err != nil {
		return nil, err
	}
	return (*nodeService)(s.partitions[part].nodes[0]).Validate(ctx, envelope, opts)
}

// Sequence routes the source to a partition and calls Sequence on the first
// node of that partition, returning the result.
func (s *simService) Sequence(ctx context.Context, src, dst *url.URL, num uint64) (*api.TransactionRecord, error) {
	part, err := s.router.RouteAccount(src)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return (*nodeService)(s.partitions[part].nodes[0]).Private().Sequence(ctx, src, dst, num)
}
