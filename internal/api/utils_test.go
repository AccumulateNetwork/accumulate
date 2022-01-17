package api_test

import (
	"testing"

	"github.com/AccumulateNetwork/accumulate/internal/accumulated"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
)

func startBVC(t *testing.T, dir string) *accumulated.Daemon {
	t.Helper()
	acctesting.SkipPlatformCI(t, "darwin", "requires setting up localhost aliases")

	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	acctesting.RunTestNet(t, subnets, daemons)
	return daemons[subnets[1]][0]
}
