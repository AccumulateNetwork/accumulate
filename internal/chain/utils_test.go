package chain

// Whitebox testing utilities

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

func NewStateManagerForTest(t *testing.T, db *database.Database, envelope *protocol.Envelope) (*StateManager, *Delivery) {
	delivery, err := NormalizeEnvelope(envelope)
	require.NoError(t, err)
	require.Len(t, delivery, 1)
	batch := db.Begin(false)
	defer batch.Discard()
	_, err = delivery[0].LoadTransaction(batch)
	require.NoError(t, err)

	txid := types.Bytes(delivery[0].Transaction.GetHash()).AsBytes32()
	m := new(StateManager)
	m.SignatorUrl = delivery[0].Signatures[0].GetSigner()
	m.OriginUrl = delivery[0].Transaction.Header.Principal
	m.stateCache = *newStateCache(protocol.SubnetUrl(t.Name()), delivery[0].Transaction.Body.Type(), txid, db.Begin(true))

	require.NoError(t, m.LoadUrlAs(m.SignatorUrl, &m.Signator))
	require.NoError(t, m.LoadUrlAs(m.OriginUrl, &m.Origin))
	return m, delivery[0]
}
