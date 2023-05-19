// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

type Partition struct {
	ID string
	tb testing.TB
	s  *simulator.Simulator
	h  *harness.Sim

	Database database.Beginner
	API      *client.Client
	Executor struct {
		Key      []byte
		Describe config.Describe
	}

	database.Beginner
}

func (s *Simulator) Partition(part string) *Partition {
	p := new(Partition)
	p.ID, p.tb, p.s, p.h = part, s.TB, s.S, s.H
	p.Database = s.S.Database(part)
	p.API = s.S.ClientV2(part)
	p.Executor.Key = s.S.SignWithNode(part, 0).Key()
	p.Executor.Describe.PartitionId = part
	p.Beginner = p.Database
	return p
}

func (s *Simulator) PartitionFor(u *url.URL) *Partition {
	s.TB.Helper()
	part, err := s.S.Router().RouteAccount(u)
	require.NoError(s.TB, err)
	return s.Partition(part)
}

func (p *Partition) Globals() *core.GlobalValues {
	p.tb.Helper()
	ns, err := p.s.Services().NetworkStatus(context.Background(), api.NetworkStatusOptions{Partition: protocol.Directory})
	require.NoError(p.tb, err)
	return &core.GlobalValues{
		Oracle:          ns.Oracle,
		Globals:         ns.Globals,
		Network:         ns.Network,
		Routing:         ns.Routing,
		ExecutorVersion: ns.ExecutorVersion,
	}
}
