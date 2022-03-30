package chain

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

func (e *Executor) ForceCommit() ([]byte, error) {
	return e.commit(true)
}

func NewStateManagerForTest(t *testing.T, db *database.Database, envelope *protocol.Envelope) *StateManager {
	txid := types.Bytes(envelope.Transaction.GetHash()).AsBytes32()
	m := new(StateManager)
	m.SignatorUrl = envelope.Signatures[0].GetSigner()
	m.OriginUrl = envelope.Transaction.Header.Principal
	m.stateCache = *newStateCache(protocol.SubnetUrl(t.Name()), envelope.Transaction.Body.Type(), txid, db.Begin(true))

	require.NoError(t, m.LoadUrlAs(m.SignatorUrl, &m.Signator))
	require.NoError(t, m.LoadUrlAs(m.OriginUrl, &m.Origin))
	return m
}
