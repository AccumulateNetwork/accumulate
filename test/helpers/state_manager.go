package helpers

import (
	"strings"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type T interface {
	require.TestingT
	Helper()
	Name() string
}

func NewStateManager(testName string, db database.Beginner, transaction *protocol.Transaction) *chain.StateManager {
	net := &config.Describe{PartitionId: strings.ReplaceAll(strings.ReplaceAll(testName, "/", "-"), "#", "-")}
	return chain.NewStateManager(net, nil, db.Begin(true), nil, transaction, nil)
}

func LoadStateManager(t T, db database.Beginner, transaction *protocol.Transaction) *chain.StateManager {
	t.Helper()
	m := NewStateManager(t.Name(), db, transaction)
	require.NoError(t, m.LoadUrlAs(m.OriginUrl, &m.Origin))
	return m
}
