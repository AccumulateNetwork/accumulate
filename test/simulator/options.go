// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator/consensus"
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
)

type Options struct {
	network     *accumulated.NetworkInit
	database    OpenDatabaseFunc
	snapshot    SnapshotFunc
	application func(node *Node, exec execute.Executor) (consensus.App, error)
	recordings  RecordingFunc

	dropDispatchedMessages bool
	skipProposalCheck      bool
	ignoreDeliverResults   bool
	ignoreCommitResults    bool
	deterministic          bool
}

type Option func(opts *Options) error

type OpenDatabaseFunc = func(partition string, node int, logger log.Logger) keyvalue.Beginner
type SnapshotFunc = func(partition string, network *accumulated.NetworkInit, logger log.Logger) (ioutil2.SectionReader, error)
type RecordingFunc = func(partition string, node int) (io.WriteSeeker, error)

// Deterministic attempts to run the simulator in a fully deterministic,
// repeatable way.
func Deterministic(opts *Options) error {
	opts.deterministic = true
	return nil
}

// DropDispatchedMessages drops all internally dispatched messages.
func DropDispatchedMessages(opts *Options) error {
	opts.dropDispatchedMessages = true
	return nil
}

// SkipProposalCheck skips checking if each non-leader node agrees with the
// leader's proposed block.
func SkipProposalCheck(opts *Options) error {
	opts.skipProposalCheck = true
	return nil
}

// IgnoreDeliverResults ignores inconsistencies in the result of DeliverTx.
func IgnoreDeliverResults(opts *Options) error {
	opts.ignoreDeliverResults = true
	return nil
}

// IgnoreCommitResults ignores inconsistencies in the result of Commit.
func IgnoreCommitResults(opts *Options) error {
	opts.ignoreCommitResults = true
	return nil
}

func WithNetwork(net *accumulated.NetworkInit) Option {
	return func(opts *Options) error {
		opts.network = net
		return nil
	}
}

func WithDatabase(fn OpenDatabaseFunc) Option {
	return func(opts *Options) error {
		opts.database = fn
		return nil
	}
}

func WithSnapshot(fn SnapshotFunc) Option {
	return func(opts *Options) error {
		opts.snapshot = fn
		return nil
	}
}

func WithApplication(fn func(*Node, execute.Executor) (consensus.App, error)) Option {
	return func(opts *Options) error {
		opts.application = fn
		return nil
	}
}

// WithRecordings takes a function that returns files to write node recordings to.
func WithRecordings(fn RecordingFunc) Option {
	return func(opts *Options) error {
		opts.recordings = fn
		return nil
	}
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

// TODO Deprecated: This is a no-op
func MemoryDatabase(*Options) error { return nil }

func BadgerDatabaseFromDirectory(dir string, onErr func(error)) Option {
	return WithDatabase(func(partition string, node int, _ log.Logger) keyvalue.Beginner {
		err := os.MkdirAll(dir, 0700)
		if err != nil {
			onErr(err)
			panic(err)
		}

		db, err := badger.New(filepath.Join(dir, fmt.Sprintf("%s-%d.db", partition, node)))
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

func Genesis(time time.Time) Option {
	// By default run tests with the new executor version
	return GenesisWithVersion(time, protocol.ExecutorVersionLatest)
}

func GenesisWithVersion(time time.Time, version protocol.ExecutorVersion) Option {
	values := new(core.GlobalValues)
	values.ExecutorVersion = version
	return GenesisWith(time, values)
}

func GenesisWith(time time.Time, values *core.GlobalValues) Option {
	return WithSnapshot(genesis(time, values))
}

func genesis(time time.Time, values *core.GlobalValues) SnapshotFunc {
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
