// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
)

type Option interface {
	apply(*simFactory) error
}

type OpenDatabaseFunc = func(partition *protocol.PartitionInfo, node int, logger log.Logger) keyvalue.Beginner
type SnapshotFunc = func(partition string, network *accumulated.NetworkInit, logger log.Logger) (ioutil2.SectionReader, error)
type RecordingFunc = func(partition string, node int) (io.WriteSeeker, error)

type optionFunc func(*simFactory) error

func (fn optionFunc) apply(f *simFactory) error { return fn(f) }

func WithLogger(logger log.Logger) Option {
	return optionFunc(func(f *simFactory) error {
		f.logger = logger
		return nil
	})
}

// Deterministic attempts to run the simulator in a fully deterministic,
// repeatable way.
func Deterministic() Option {
	return optionFunc(func(opts *simFactory) error {
		opts.deterministic = true
		return nil
	})
}

// DropDispatchedMessages drops all internally dispatched messages.
func DropDispatchedMessages() Option {
	return optionFunc(func(opts *simFactory) error {
		opts.dropDispatchedMessages = true
		opts.dropInitialAnchor = true
		opts.disableAnchorHealing = true
		return nil
	})
}

// DropInitialAnchor drops anchors when they are initially submitted.
func DropInitialAnchor() Option {
	return optionFunc(func(opts *simFactory) error {
		opts.dropInitialAnchor = true
		return nil
	})
}

// DisableAnchorHealing disables healing of anchors after they are initially
// submitted.
func DisableAnchorHealing() Option {
	return optionFunc(func(opts *simFactory) error {
		opts.disableAnchorHealing = true
		return nil
	})
}

// CaptureDispatchedMessages allows the caller to capture internally dispatched
// messages.
func CaptureDispatchedMessages(fn DispatchInterceptor) Option {
	return optionFunc(func(opts *simFactory) error {
		opts.interceptDispatchedMessages = fn
		return nil
	})
}

// SkipProposalCheck skips checking if each non-leader node agrees with the
// leader's proposed block.
func SkipProposalCheck() Option {
	return optionFunc(func(opts *simFactory) error {
		opts.skipProposalCheck = true
		return nil
	})
}

// IgnoreDeliverResults ignores inconsistencies in the result of DeliverTx.
func IgnoreDeliverResults() Option {
	return optionFunc(func(opts *simFactory) error {
		opts.ignoreDeliverResults = true
		return nil
	})
}

// IgnoreCommitResults ignores inconsistencies in the result of Commit.
func IgnoreCommitResults() Option {
	return optionFunc(func(opts *simFactory) error {
		opts.ignoreCommitResults = true
		return nil
	})
}

func WithNetwork(net *accumulated.NetworkInit) Option {
	return optionFunc(func(opts *simFactory) error {
		opts.network = net
		return nil
	})
}

func WithDatabase(fn OpenDatabaseFunc) Option {
	return optionFunc(func(opts *simFactory) error {
		opts.storeOpt = fn
		return nil
	})
}

func WithSnapshot(fn SnapshotFunc) Option {
	return optionFunc(func(opts *simFactory) error {
		opts.snapshot = fn
		return nil
	})
}

// WithRecordings takes a function that returns files to write node recordings to.
func WithRecordings(fn RecordingFunc) Option {
	return optionFunc(func(opts *simFactory) error {
		opts.recordings = fn
		return nil
	})
}

func SimpleNetwork(name string, bvnCount, nodeCount int) Option {
	return WithNetwork(NewSimpleNetwork(name, bvnCount, nodeCount))
}

// NewSimpleNetwork creates a basic network with the given name, number of BVNs,
// and number of nodes per BVN.
func NewSimpleNetwork(name string, bvnCount, nodeCount int) *accumulated.NetworkInit {
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

func LocalNetwork(name string, bvnCount, nodeCount int, baseIP net.IP, basePort uint64) Option {
	return WithNetwork(NewLocalNetwork(name, bvnCount, nodeCount, baseIP, basePort))
}

// NewLocalNetwork returns a SimpleNetwork with sequential IPs starting from the
// base IP with the given base port.
func NewLocalNetwork(name string, bvnCount, nodeCount int, baseIP net.IP, basePort uint64) *accumulated.NetworkInit {
	net := NewSimpleNetwork(name, bvnCount, nodeCount)
	for _, bvn := range net.Bvns {
		for _, node := range bvn.Nodes {
			node.AdvertizeAddress = baseIP.String()
			node.BasePort = basePort
			baseIP[len(baseIP)-1]++
		}
	}
	return net
}

// MemoryDatabase configures the simulator to use in-memory databases.
//
// Deprecated: This is a no-op
func MemoryDatabase(*simFactory) error { return nil }

func BadgerDatabaseFromDirectory(dir string, onErr func(error)) Option {
	return WithDatabase(func(partition *protocol.PartitionInfo, node int, _ log.Logger) keyvalue.Beginner {
		err := os.MkdirAll(dir, 0700)
		if err != nil {
			onErr(err)
			panic(err)
		}

		db, err := badger.New(filepath.Join(dir, fmt.Sprintf("%s-%d.db", partition.ID, node)))
		if err != nil {
			onErr(err)
			panic(err)
		}

		return db
	})
}

func SnapshotFromDirectory(dir string) Option {
	return WithSnapshot(func(partition string, network *accumulated.NetworkInit, logger log.Logger) (ioutil2.SectionReader, error) {
		return os.Open(filepath.Join(dir, fmt.Sprintf("%s.snapshot", partition)))
	})
}

func SnapshotMap(snapshots map[string][]byte) Option {
	return WithSnapshot(func(partition string, _ *accumulated.NetworkInit, _ log.Logger) (ioutil2.SectionReader, error) {
		return ioutil2.NewBuffer(snapshots[partition]), nil
	})
}

func Genesis(time time.Time) genesis {
	// By default run tests with the new executor version
	return genesis{time: time}
}

// DO NOT USE - use Genesis(time).WithVersion(version)
func GenesisWithVersion(time time.Time, version protocol.ExecutorVersion) genesis {
	return Genesis(time).WithVersion(version)
}

// DO NOT USE - use Genesis(time).WithValues(values)
func GenesisWith(time time.Time, values *network.GlobalValues) genesis {
	return Genesis(time).WithValues(values)
}

type genesis struct {
	time   time.Time
	values *network.GlobalValues
	extra  []DbBuilder
}

func (g genesis) WithValues(values *network.GlobalValues) genesis {
	g.values = values
	return g
}

func (g genesis) WithVersion(version protocol.ExecutorVersion) genesis {
	if g.values == nil {
		g.values = new(network.GlobalValues)
	}
	g.values.ExecutorVersion = version
	return g
}

type DbBuilder interface {
	Build(func(*url.URL) database.Updater) error
}

func (g genesis) With(builders ...DbBuilder) genesis {
	g.extra = append(g.extra, builders...)
	return g
}

func (g genesis) apply(opts *simFactory) error {
	if g.values == nil {
		g.values = new(network.GlobalValues)
		g.values.ExecutorVersion = protocol.ExecutorVersionLatest
	}

	var genDocs map[string][]byte
	opts.snapshot = func(partition string, net *accumulated.NetworkInit, logger log.Logger) (ioutil2.SectionReader, error) {
		if genDocs != nil {
			return ioutil2.NewBuffer(genDocs[partition]), nil
		}

		var snapshots []func(*network.GlobalValues) (ioutil.SectionReader, error)
		if len(g.extra) > 0 {
			var extra []byte
			snapshots = append(snapshots, func(globals *network.GlobalValues) (ioutil.SectionReader, error) {
				if extra != nil {
					return ioutil.NewBuffer(extra), nil
				}

				db := database.OpenInMemory(nil)
				db.SetObserver(execute.NewDatabaseObserver())

				// Fake the system ledger
				err := db.Update(func(batch *database.Batch) error {
					ledger := &protocol.SystemLedger{
						Url:             protocol.DnUrl().JoinPath(protocol.Ledger),
						ExecutorVersion: globals.ExecutorVersion,
					}
					return batch.Account(ledger.Url).Main().Put(ledger)
				})
				if err != nil {
					return nil, err
				}

				for _, b := range g.extra {
					err := b.Build(func(*url.URL) database.Updater { return db })
					if err != nil {
						return nil, err
					}
				}

				buf := new(ioutil.Buffer)
				_, err = db.Collect(buf, nil, &database.CollectOptions{
					Predicate: func(r record.Record) (bool, error) {
						// Do not create a BPT
						if r.Key().Get(0) == "BPT" {
							return false, nil
						}
						return true, nil
					},
				})
				extra = buf.Bytes()
				return ioutil.NewBuffer(extra), err
			})
		}

		var err error
		genDocs, err = accumulated.BuildGenesisDocs(net, g.values, g.time, logger, nil, snapshots)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("build genesis docs: %w", err)
		}

		return ioutil2.NewBuffer(genDocs[partition]), nil
	}
	return nil
}

// InitialAcmeSupply overrides the default initial ACME supply. A value of nil
// will disable setting the initial supply.
func InitialAcmeSupply(v *big.Int) Option {
	return optionFunc(func(f *simFactory) error {
		f.initialSupply = v
		return nil
	})
}

func UseABCI() Option {
	return optionFunc(func(opts *simFactory) error {
		opts.abci = withABCI
		return nil
	})
}
