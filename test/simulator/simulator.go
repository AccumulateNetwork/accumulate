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

	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/sync/errgroup"
)

type Simulator struct {
	logger     logging.OptionalLogger
	init       *accumulated.NetworkInit
	database   OpenDatabaseFunc
	partitions map[string]*Partition
	router     *Router
	netcfg     *config.Network
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
				s.netcfg.Partitions[i+1].Nodes[j].Address = node.Address(accumulated.AdvertizeAddress, "http", config.PortOffsetBlockValidator)
				s.netcfg.Partitions[0].Nodes[j].Address = node.Address(accumulated.AdvertizeAddress, "http", config.PortOffsetDirectory)
			}
		}
	}

	var err error
	s.partitions[protocol.Directory], err = newDn(s, network)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	for _, bvn := range network.Bvns {
		s.partitions[bvn.Id], err = newBvn(s, bvn)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknownError, err)
		}
	}

	events.SubscribeSync(s.partitions[protocol.Directory].nodes[0].eventBus, s.router.willChangeGlobals)

	for _, p := range s.partitions {
		snapshot, err := snapshot(p.ID, s.init, s.logger)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "open snapshot: %w", err)
		}
		err = p.initChain(snapshot)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "init %s: %w", p.ID, err)
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
				PrivValKey: testing.GenerateKey(name, bvnInit.Id, j),
			})
		}
		net.Bvns = append(net.Bvns, bvnInit)
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
				return nil, errors.Format(errors.StatusUnknownError, "build genesis docs: %w", err)
			}
		}

		var snapshot []byte
		err = json.Unmarshal(genDocs[partition].AppState, &snapshot)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknownError, err)
		}

		return ioutil2.NewBuffer(snapshot), nil
	}
}

// Step executes a single simulator step
func (s *Simulator) Step() error {
	errg := new(errgroup.Group)
	for _, p := range s.partitions {
		p := p // Don't capture loop variables
		errg.Go(func() error { return p.execute(errg) })
	}
	return errg.Wait()
}

func (s *Simulator) SetSubmitHook(partition string, fn SubmitHookFunc) {
	s.partitions[partition].SetSubmitHook(fn)
}

func (s *Simulator) Submit(delivery *chain.Delivery) (*protocol.TransactionStatus, error) {
	partition, err := s.router.Route(&protocol.Envelope{
		Transaction: []*protocol.Transaction{delivery.Transaction},
		Signatures:  delivery.Signatures,
	})
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	p, ok := s.partitions[partition]
	if !ok {
		return nil, errors.Format(errors.StatusBadRequest, "%s is not a partition", partition)
	}

	return p.Submit(delivery, false)
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

func (s *Simulator) Database(partition string) database.Updater {
	p, ok := s.partitions[partition]
	if !ok {
		return errDb{errors.Format(errors.StatusBadRequest, "%s is not a partition", partition)}
	}
	return p
}

func (s *Simulator) DatabaseFor(account *url.URL) database.Updater {
	partition, err := s.router.RouteAccount(account)
	if err != nil {
		return errDb{errors.Wrap(errors.StatusUnknownError, err)}
	}
	return s.Database(partition)
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

func (s *Simulator) ListenAndServe(hook func(*Simulator, http.Handler) http.Handler) error {
	errg := new(errgroup.Group)
	for _, part := range s.partitions {
		for _, node := range part.nodes {
			var addr string
			if part.Type == config.Directory {
				addr = node.init.Address(accumulated.ListenAddress, "", config.PortOffsetDirectory, config.PortOffsetAccumulateApi)
			} else {
				addr = node.init.Address(accumulated.ListenAddress, "", config.PortOffsetBlockValidator, config.PortOffsetAccumulateApi)
			}

			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}
			defer func() { _ = ln.Close() }()

			srv := http.Server{Handler: node.api.NewMux()}
			if hook != nil {
				srv.Handler = hook(s, srv.Handler)
			}

			defer func() { _ = srv.Shutdown(context.Background()) }()
			errg.Go(func() error { return srv.Serve(ln) })

			s.logger.Info("Node up", "partition", part.ID, "node", node.id, "address", "http://"+addr)
		}
	}
	return errg.Wait()
}
