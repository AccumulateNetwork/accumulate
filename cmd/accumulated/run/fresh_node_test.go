// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A node that has only loaded genesis has executed nothing, so it must not
// join: it is the first node of a network and has nobody to ask (executor
// spec, "Sync", step 2).
//
// This is the daemon's own computation, against a database the daemon's own
// genesis loader filled. Nothing is passed by hand: the snapshot is built by
// genesis.Init the way netsim builds it, loaded by
// DAGBFTService.loadGenesisIfNeeded, read back by lastExecutedBlock and judged
// by nodeMustJoin — the four steps `start` runs in that order. The gap #4304
// names is exactly the one between them: the join's own test supplied the
// verdict instead of computing it, so nothing ever noticed that genesis
// writes block 1 and that `lastBlock > 0` is therefore true of every node
// that has ever started.
func TestAGenesisLoadedNodeDoesNotJoin(t *testing.T) {
	dir := t.TempDir()
	path := writeGenesisSnapshot(t, dir, protocol.Directory, protocol.PartitionTypeDirectory)

	store := memory.New(nil)
	logger := logging.NewSlogLogger(slog.Default())
	db := database.New(store, logger)

	svc := &DAGBFTService{Partition: &protocol.PartitionInfo{ID: protocol.Directory, Type: protocol.PartitionTypeDirectory}}
	loaded, err := svc.loadGenesisIfNeeded(db, path, logger)
	require.NoError(t, err)
	require.True(t, loaded, "the database was empty, so genesis was loaded")

	last, err := lastExecutedBlock(db, protocol.Directory)
	require.NoError(t, err)
	require.Equal(t, uint64(protocol.GenesisBlock), last,
		"loading genesis writes the ledger at the genesis block, which is why `lastBlock > 0` was always true")

	require.False(t, nodeMustJoin(last),
		"a node that has executed nothing does not join: there is nobody to ask, and nothing to be exact about")
}

// And the converse, so that "never join" cannot pass for a fix: once a block
// of this node's own has been executed, it must join.
//
// The block is written rather than executed — the ledger index is what a
// block's commit advances, and this writes it — because what is under test is
// the daemon's reading of the ledger, not the executor. A node that has
// executed a block on a real network is the restart case, and the netsim test
// beside this one covers the fresh-network case end to end.
func TestANodeThatExecutedABlockJoins(t *testing.T) {
	dir := t.TempDir()
	path := writeGenesisSnapshot(t, dir, protocol.Directory, protocol.PartitionTypeDirectory)

	store := memory.New(nil)
	logger := logging.NewSlogLogger(slog.Default())
	db := database.New(store, logger)

	svc := &DAGBFTService{Partition: &protocol.PartitionInfo{ID: protocol.Directory, Type: protocol.PartitionTypeDirectory}}
	_, err := svc.loadGenesisIfNeeded(db, path, logger)
	require.NoError(t, err)

	batch := db.Begin(true)
	defer batch.Discard()
	account := batch.Account(protocol.DnUrl().JoinPath(protocol.Ledger))
	var ledger *protocol.SystemLedger
	require.NoError(t, account.Main().GetAs(&ledger))
	ledger.Index = protocol.GenesisBlock + 1
	require.NoError(t, account.Main().Put(ledger))
	require.NoError(t, batch.Commit())

	last, err := lastExecutedBlock(db, protocol.Directory)
	require.NoError(t, err)
	require.Equal(t, uint64(protocol.GenesisBlock+1), last)

	require.True(t, nodeMustJoin(last),
		"a node that has executed a block of its own takes a peer's staging before it executes another")
}

// writeGenesisSnapshot builds a real genesis document, the way netsim does.
func writeGenesisSnapshot(t *testing.T, dir, partition string, typ protocol.PartitionType) string {
	t.Helper()

	def := new(protocol.NetworkDefinition)
	def.NetworkName = "FreshNode"
	def.AddPartition(protocol.Directory, protocol.PartitionTypeDirectory)
	def.AddPartition("BVN1", protocol.PartitionTypeBlockValidator)

	key := make([]byte, 32)
	key[0] = 1
	def.AddValidator(key, protocol.Directory, true)
	def.AddValidator(key, "BVN1", true)

	globals := new(core.GlobalValues)
	globals.Network = def
	globals.Globals = &protocol.NetworkGlobals{BlockInterval: time.Second}

	path := filepath.Join(dir, partition+"-genesis.snap")
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	err = genesis.Init(f, genesis.InitOpts{
		NetworkID:      def.NetworkName,
		PartitionId:    partition,
		NetworkType:    typ,
		GenesisTime:    time.Now(),
		Logger:         logging.NewSlogLogger(slog.Default()),
		GenesisGlobals: globals,
		OperatorKeys:   [][]byte{key},
	})
	require.NoError(t, err)
	return path
}

// A fresh network starts and produces blocks. This is the whole daemon:
// netsim writes the genesis documents, `New`/`Start` run the same
// DAGBFTService.start a deployed node runs, genesis is loaded there, the
// predicate is computed there, and block 2 is read back over the node's own
// HTTP API. Nothing is passed by hand — which is the point, because the flag
// #4304 is about was only ever true in a test that supplied it.
//
// What this fails on, without the fix: the node reads its ledger at block 1,
// decides it must join, starts collecting, and asks the partition's
// validators for staging — of which it is one, because join.APIPeers looks up
// whoever serves the sequencer and the node serves it itself. It refuses
// itself, ten times, two seconds apart, and only then gives up and executes.
// Measured: 18 seconds to block 2, against 1 second with the fix.
//
// So the deadline here is the join's own budget, and it is the assertion.
// #4304's correction is precisely that a network which starts because every
// node timed out has not started deterministically — on twelve nodes that is
// a race whose winner is one arbitrary node per partition, and the other
// eleven then join from it.
//
// ONE VALIDATOR, and that is a limit of the harness rather than a choice: a
// netsim's subnodes share the parent's libp2p host (subnode.go, `sub.p2p =
// inst.p2p`), so two in-process validators are one peer and consensus never
// leaves round 0. Every netsim test in this package is Validators: 1 for that
// reason. So the twelve-node shape from run 20260918T131713Z — all of them
// joining, all of them refusing each other the staging none of them had — is
// Docker's to prove, not this test's. What this proves is the node-level
// property underneath it: a node that has executed nothing does not join, and
// a network of such nodes therefore starts by executing rather than by
// waiting for each other.
func TestAFreshNetworkStartsWithoutJoining(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a network")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rootDir := t.TempDir()
	basePort := freeDevnetBase(t, 1)

	interval := encoding.Duration(time.Second)
	cfg := &Config{
		Network: "FreshStart",
		Logging: &Logging{
			Format: "plain",
			Rules:  []*LoggingRule{{Level: slog.LevelError}},
		},
		P2P: &P2P{
			Key: &PrivateKeySeed{Seed: record.NewKey("fresh-network-starts")},
		},
		Configurations: []Configuration{
			&NetSimConfiguration{
				Listen:        multiaddr.StringCast(fmt.Sprintf("/tcp/%d", basePort)),
				Bvns:          1,
				Validators:    1,
				BlockInterval: &interval,
				Globals: &network.GlobalValues{
					Globals: &protocol.NetworkGlobals{
						// Never during the test.
						MajorBlockSchedule: "0 0 1 1 *",
					},
				},
			},
		},
	}
	cfg.file = filepath.Join(rootDir, "accumulate.toml")

	inst, err := New(ctx, cfg)
	require.NoError(t, err)
	inst.rootDir = rootDir
	require.NoError(t, inst.Start())
	defer inst.Stop()

	api := fmt.Sprintf("http://127.0.0.1:%d/v3", basePort+int(portAccAPI))
	parts := []string{"dn", "bvn-BVN1"}

	// The clock starts when the API answers, so node start-up is not counted
	// against block production.
	var ready bool
	for i := 0; i < 60 && !ready; i++ {
		ready = true
		for _, p := range parts {
			if _, err := ledgerIndex(api, p); err != nil {
				ready = false
			}
		}
		if !ready {
			time.Sleep(time.Second)
		}
	}
	require.True(t, ready, "the API never became ready")

	// Ten of the join's own retries: far more than block production needs
	// (one second, measured), and a node that had wrongly entered the join
	// would still be pulling at the end of it.
	deadline := 10 * join.DefaultRetry
	height := map[string]uint64{}
	done := false
	for start := time.Now(); time.Since(start) < deadline && !done; {
		done = true
		for _, p := range parts {
			h, err := ledgerIndex(api, p)
			if err != nil {
				done = false
				continue
			}
			height[p] = h
			if h < protocol.GenesisBlock+1 {
				done = false
			}
		}
		if !done {
			time.Sleep(250 * time.Millisecond)
		}
	}
	require.True(t, done,
		"a fresh network must reach block %d on every partition within %v, by executing rather than by giving up on a join; heights were %v",
		protocol.GenesisBlock+1, deadline, height)
}
