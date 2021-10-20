package node_test

import (
	"net"
	"testing"

	"github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/testing/e2e"
	"github.com/stretchr/testify/suite"
)

func TestEndToEnd(t *testing.T) {
	nodes := initNodes(t, net.ParseIP("127.0.25.1"), 3000, 3, "error")
	query := startNodes(t, nodes)

	suite.Run(t, e2e.NewSuite(func(s *e2e.Suite) *api.Query {
		return query
	}))
}
