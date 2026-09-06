// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// Memory under cross-partition load, in-process, on the BlockchainDB store
// (soak 20260905T235425Z: 103 -> 1300 MiB RSS on a BVN in seven minutes at
// 500 tps). Set ACC_MEM_PROFILE=<path> to write a heap profile at the end;
// ACC_MEM_BLOCKS and ACC_MEM_PER_BLOCK size the load. The test reports heap
// in use after the load and after a forced collection; it is a measurement,
// not an assertion, until a budget is set.
func TestMemoryUnderLoad(t *testing.T) {
	profile := os.Getenv("ACC_MEM_PROFILE")
	if profile == "" {
		t.Skip("measurement only; set ACC_MEM_PROFILE=<heap profile path> to run")
	}
	blocks, perBlock := 200, 100
	if v, err := strconv.Atoi(os.Getenv("ACC_MEM_BLOCKS")); err == nil {
		blocks = v
	}
	if v, err := strconv.Atoi(os.Getenv("ACC_MEM_PER_BLOCK")); err == nil {
		perBlock = v
	}

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 1),
		simulator.GenesisWith(GenesisTime, globals),
		simulator.WithDatabase(simulator.BcdbDbOpener(t.TempDir(), func(err error) { require.NoError(t, err) })),
	)

	// Senders on one partition, receivers on the other, so every transfer
	// produces a cross-partition synthetic
	const senders = 20
	type sender struct {
		key []byte
		url *url.URL
		ts  uint64
	}
	var from []*sender
	var to []*url.URL
	part0 := ""
	for i := 0; len(from) < senders || len(to) < senders; i++ {
		key := acctesting.GenerateKey("mem", i)
		u := acctesting.AcmeLiteAddressStdPriv(key)
		part, err := sim.Router().RouteAccount(u)
		require.NoError(t, err)
		if part0 == "" {
			part0 = part
		}
		if part == part0 && len(from) < senders {
			MakeLiteTokenAccount(t, sim.DatabaseFor(u), key[32:], AcmeUrl())
			from = append(from, &sender{key: key, url: u})
		} else if part != part0 && len(to) < senders {
			to = append(to, u)
		}
	}

	var before runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	for b := 0; b < blocks; b++ {
		for i := 0; i < perBlock; i++ {
			s := from[i%senders]
			sim.SubmitSuccessfully(MustBuild(t,
				build.Transaction().For(s.url).
					SendTokens(1, 0).To(to[(i/senders)%senders]).
					SignWith(s.url).Version(1).Timestamp(&s.ts).PrivateKey(s.key)))
		}
		sim.StepN(1)
	}
	sim.StepN(30)

	var after, collected runtime.MemStats
	runtime.ReadMemStats(&after)
	f, err := os.Create(profile)
	require.NoError(t, err)
	runtime.GC()
	require.NoError(t, pprof.WriteHeapProfile(f))
	require.NoError(t, f.Close())
	runtime.ReadMemStats(&collected)
	t.Logf("%d blocks x %d transfers: heap in use before %d MiB, after load %d MiB, after GC %d MiB; profile at %s",
		blocks, perBlock, before.HeapInuse>>20, after.HeapInuse>>20, collected.HeapInuse>>20, profile)
	_ = protocol.AcmePrecision
}
