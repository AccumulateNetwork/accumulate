package harness

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

var GenesisTime = time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC)

func NewSim(tb testing.TB, database simulator.OpenDatabaseFunc, init *accumulated.NetworkInit, snapshot simulator.SnapshotFunc) *Sim {
	var err error
	s := new(Sim)
	s.tb = tb
	s.s, err = simulator.New(acctesting.NewTestLogger(tb), database, init, snapshot)
	require.NoError(tb, err)
	s.services = s.s.Services()
	s.stepper = s.s
	return s
}

type Sim struct {
	Harness
	s *simulator.Simulator
}

func (s *Sim) Router() routing.Router {
	return s.s.Router()
}

func (s *Sim) Partitions() []*protocol.PartitionInfo {
	return s.s.Partitions()
}

func (s *Sim) Database(partition string) database.Updater {
	return s.s.Database(partition)
}

func (s *Sim) DatabaseFor(account *url.URL) database.Updater {
	return s.s.DatabaseFor(account)
}

func (s *Sim) SetRoute(account *url.URL, partition string) {
	s.s.SetRoute(account, partition)
}

func (s *Sim) SetSubmitHook(partition string, fn simulator.SubmitHookFunc) {
	s.s.SetSubmitHook(partition, fn)
}

func (s *Sim) SignWithNode(partition string, i int) signing.Signer {
	return s.s.SignWithNode(partition, i)
}
