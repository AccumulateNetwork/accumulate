package node_test

import (
	"net"
	"os"
	"runtime"
	"testing"

	"github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/testing/e2e"
	"github.com/stretchr/testify/suite"
)

func TestEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("This test does not work well on Windows or macOS")
	}

	if os.Getenv("CI") == "true" {
		t.Skip("This test consistently fails in CI")
	}

	suite.Run(t, e2e.NewSuite(func(s *e2e.Suite) *api.Query {
		// Restart the nodes for every test
		nodes := initNodes(s.T(), net.ParseIP("127.0.25.1"), 3000, 3, "error")
		query := startNodes(s.T(), nodes)
		return query
	}))
}
