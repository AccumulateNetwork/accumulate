// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package genesis_test

import (
	"crypto/ed25519"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4585: genesis must not open a major block, whatever second of the
// schedule it falls on. A major block opens at the first block whose time
// plus one second reaches the schedule's next time; a genesis in the last
// second before a scheduled time used to open that major block itself, so
// the network booted with major block 1 already open and the first boundary
// after boot had none.
func TestGenesisOpensNoMajorBlock(t *testing.T) {
	for _, at := range []string{"12:00:30.000", "12:00:59.000", "12:00:59.500", "12:00:59.999"} {
		t.Run(at, func(t *testing.T) {
			genesisTime, err := time.Parse("2006-01-02 15:04:05.000", "2026-10-15 "+at)
			require.NoError(t, err)

			logger := logging.NewSlogLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
			key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
			def := new(protocol.NetworkDefinition)
			def.NetworkName = "test"
			def.AddPartition(protocol.Directory, protocol.PartitionTypeDirectory)
			def.AddPartition("BVN1", protocol.PartitionTypeBlockValidator)
			def.AddValidator(key.Public().(ed25519.PublicKey), protocol.Directory, true)
			def.AddValidator(key.Public().(ed25519.PublicKey), "BVN1", true)
			globals := network.NewGlobals(&network.GlobalValues{
				ExecutorVersion: protocol.ExecutorVersionLatest,
				Globals:         &protocol.NetworkGlobals{MajorBlockSchedule: "* * * * *"},
				Network:         def,
			})

			buf := new(ioutil2.Buffer)
			err = genesis.Init(buf, genesis.InitOpts{
				NetworkID:      "test",
				PartitionId:    protocol.Directory,
				NetworkType:    protocol.PartitionTypeDirectory,
				GenesisTime:    genesisTime,
				GenesisGlobals: globals,
				OperatorKeys:   [][]byte{key.Public().(ed25519.PublicKey)},
				Logger:         logger,
			})
			require.NoError(t, err)

			db := database.OpenInMemory(logger)
			err = snapshot.FullRestore(db, ioutil2.NewBuffer(buf.Bytes()), logger,
				config.NetworkUrl{URL: protocol.DnUrl()})
			require.NoError(t, err)

			batch := db.Begin(false)
			defer batch.Discard()
			var anchor *protocol.AnchorLedger
			require.NoError(t, batch.Account(protocol.DnUrl().JoinPath(protocol.AnchorPool)).Main().GetAs(&anchor))
			require.Zero(t, anchor.MajorBlockIndex, "genesis at %s opened major block %d at %v", at, anchor.MajorBlockIndex, anchor.MajorBlockTime)
		})
	}
}
