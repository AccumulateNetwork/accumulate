// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
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

	// Deterministic attempts to run the simulator in a fully deterministic,
	// repeatable way
	Deterministic bool

	// DropDispatchedMessages drops all internally dispatched messages
	DropDispatchedMessages bool
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

func SnapshotMap(snapshots map[string][]byte) SnapshotFunc {
	return func(partition string, _ *accumulated.NetworkInit, _ log.Logger) (ioutil2.SectionReader, error) {
		return ioutil2.NewBuffer(snapshots[partition]), nil
	}
}

func EmptySnapshots(partition string, _ *accumulated.NetworkInit, _ log.Logger) (ioutil2.SectionReader, error) {
	return new(ioutil2.Buffer), nil
}

func Genesis(time time.Time) SnapshotFunc {
	// By default run tests with the new executor version
	return GenesisWithVersion(time, protocol.ExecutorVersionLatest)
}

func GenesisWithVersion(time time.Time, version protocol.ExecutorVersion) SnapshotFunc {
	values := new(core.GlobalValues)
	values.ExecutorVersion = version
	return GenesisWith(time, values)
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

func (s *Simulator) BlockIndex(partition string) uint64 {
	p := s.partitions[partition]
	p.loadBlockIndex()
	return p.blockIndex
}

// Step executes a single simulator step
func (s *Simulator) Step() error {
	s.blockErrGroup = new(errgroup.Group)

	if s.Deterministic {
		err := s.partitions[protocol.Directory].execute()
		for _, bvn := range s.init.Bvns {
			if e := s.partitions[bvn.Id].execute(); e != nil {
				err = e
			}
		}
		return err
	} else {
		for _, p := range s.partitions {
			p := p // Don't capture loop variables
			s.blockErrGroup.Go(p.execute)
		}
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

func (s *Simulator) SetBlockHookFor(account *url.URL, fn BlockHookFunc) {
	partition, err := s.router.RouteAccount(account)
	if err != nil {
		panic(err)
	}
	s.partitions[partition].SetBlockHook(fn)
}

func (s *Simulator) SetBlockHook(partition string, fn BlockHookFunc) {
	s.partitions[partition].SetBlockHook(fn)
}

func (s *Simulator) Submit(messages []messaging.Message) ([]*protocol.TransactionStatus, error) {
	partition, err := routing.RouteMessages(s.router, messages)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return s.SubmitTo(partition, messages)
}

func (s *Simulator) SubmitTo(partition string, messages []messaging.Message) ([]*protocol.TransactionStatus, error) {
	p, ok := s.partitions[partition]
	if !ok {
		return nil, errors.BadRequest.WithFormat("%s is not a partition", partition)
	}

	return p.Submit(messages, false)
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
func (e errDb) SetObserver(observer database.Observer)   { panic(e.err) }

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

func (s *Simulator) ViewAll(fn func(batch *database.Batch) error) error {
	for _, p := range s.partitions {
		err := p.View(fn)
		if err != nil {
			return err
		}
	}
	return nil
}
