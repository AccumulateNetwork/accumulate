// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api_test

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/suite"
	dut "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// skipOnAndroid skips tests that are known to be problematic on Android/Termux
func skipOnAndroid(t *testing.T, reason string) {
	if runtime.GOOS == "android" || isTermux() {
		t.Skipf("Skipping on Android/Termux: %s", reason)
	}
}

// isTermux detects if we're running in Termux environment
func isTermux() bool {
	// Check for Termux-specific environment variables
	if os.Getenv("TERMUX_VERSION") != "" {
		return true
	}
	if os.Getenv("PREFIX") == "/data/data/com.termux/files/usr" {
		return true
	}
	// Check if we're in the Termux directory structure
	if _, err := os.Stat("/data/data/com.termux/files/usr"); err == nil {
		return true
	}
	return false
}

func TestNetworkService(t *testing.T) {
	skipOnAndroid(t, "known goroutine deadlocks in logging")
	suite.Run(t, new(NetworkServiceTestSuite))
}

type NetworkServiceTestSuite struct {
	suite.Suite
	sim *simulator.Simulator
}

func (s *NetworkServiceTestSuite) ServiceFor(partition string) api.NetworkService {
	return dut.NewNetworkService(dut.NetworkServiceParams{
		Logger:    acctesting.NewTestLogger(s.T()),
		Database:  s.sim.Database(partition),
		EventBus:  s.sim.EventBus(partition),
		Partition: partition,
	})
}

func (s *NetworkServiceTestSuite) SetupSuite() {
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.MajorBlockSchedule = "* * * * *"
	g.ExecutorVersion = ExecutorVersionV2

	var err error
	s.sim, err = simulator.New(
		simulator.WithLogger(acctesting.NewTestLogger(s.T())),
		simulator.SimpleNetwork(s.T().Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
	)
	s.Require().NoError(err)
}

func (s *NetworkServiceTestSuite) TestDnHeight() {
	sim := NewSimWith(s.T(), s.sim)
	for _, p := range sim.Partitions() {
		sim.StepUntil(
			DnHeight(1).OnPartition(p.ID))

		ns, err := s.ServiceFor(p.ID).NetworkStatus(context.Background(), api.NetworkStatusOptions{Partition: p.ID})
		_ = s.NoError(err) &&
			s.NotZero(ns.DirectoryHeight)
	}
}
