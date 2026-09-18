// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

// AUDIT ONLY (no production change). The gates that stop a joining node
// serving are real and tested in their own packages. What has no test is the
// DAEMON'S WIRING of them. These tests record how that wiring fails: not
// loudly, but by handing a service no gate at all.

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	api "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/dagbft"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func auditEmptyDB(t *testing.T) *database.Database {
	t.Helper()
	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())
	return db
}

// TestAudit_AMissingNodeStateIsNotAnError -- cmd/accumulated/run/api.go:28
// declares the querier's node state with ioc.Wants, which is OPTIONAL by
// construction (exp/ioc/dependency.go:23-24, :56-62). If nothing registered
// it -- a registration-order slip, a partition-key that does not match, a
// config with a querier and no consensus service -- Get returns (nil, nil).
//
// The querier then has nodeState == nil, and internal/api/v3/querier.go:151
// reads nil as "this node never joined", so it answers everything, including
// the two shapes a pull reads. The gate does not fail closed; it disappears.
func TestAudit_AMissingNodeStateIsNotAnError(t *testing.T) {
	reg := ioc.Registry{}
	q := &Querier{Partition: protocol.Directory}

	serving, err := querierWantsNodeState.Get(reg, q)
	require.NoError(t, err, "an unregistered node state is not an error")
	require.Nil(t, serving, "and the querier is handed nothing")

	// That nothing is what the querier calls its gate.
	impl := api.NewQuerier(api.QuerierParams{
		Database:  auditEmptyDB(t),
		Partition: protocol.Directory,
		NodeState: serving,
	})
	_, err = impl.Query(t.Context(), protocol.DnUrl(), &v3.BptPageQuery{Count: 4})
	require.False(t, errors.Is(err, errors.NotReady),
		"a querier with no node state refused a BPT page; the audit is out of date: %v", err)

	// The same call with a joining node's machine is refused, which is what
	// the missing registration costs.
	impl = api.NewQuerier(api.QuerierParams{
		Database:  auditEmptyDB(t),
		Partition: protocol.Directory,
		NodeState: nodestate.New(protocol.DnUrl()),
	})
	_, err = impl.Query(t.Context(), protocol.DnUrl(), &v3.BptPageQuery{Count: 4})
	require.True(t, errors.Is(err, errors.NotReady), "%v", err)
}

// TestAudit_TheQuerierGateIsDeclaredOptional states the same thing about the
// dependency graph itself: the solver is allowed to start the querier without
// a node state, and does so silently.
func TestAudit_TheQuerierGateIsDeclaredOptional(t *testing.T) {
	q := &Querier{Partition: protocol.Directory}
	var found bool
	for _, r := range q.Requires() {
		if r.Descriptor.Type() == reflect.TypeOf((*nodestate.Serving)(nil)).Elem() {
			found = true
			require.True(t, r.Optional,
				"the join gate is a hard requirement now; the audit is out of date")
		}
	}
	require.True(t, found, "the querier no longer asks for node state at all")
}

// TestAudit_ServicesRegisteredWithNoNodeStateAtAll pins which of the services
// the daemon publishes on the p2p host can even be told the node is joining.
// A service whose parameters have no NodeState field cannot be gated without
// changing it, and every one of these is registered -- and DHT-advertised --
// while the node is still BOOTING.
func TestAudit_ServicesRegisteredWithNoNodeStateAtAll(t *testing.T) {
	has := func(v any) bool {
		tp := reflect.TypeOf(v)
		for i := 0; i < tp.NumField(); i++ {
			if tp.Field(i).Name == "NodeState" {
				return true
			}
		}
		return false
	}

	// Gated: these take a nodestate.Serving.
	require.True(t, has(dagbft.SubmitterServiceParams{}), "submitter")
	require.True(t, has(dagbft.ValidatorServiceParams{}), "validator")
	require.True(t, has(api.SequencerParams{}), "sequencer")
	require.True(t, has(api.QuerierParams{}), "querier")

	// Ungated: no field to put it in. Each is registered by the daemon and
	// answers peers while the node is joining.
	require.False(t, has(dagbft.ConsensusAPIServiceParams{}),
		"the consensus API service is gated now; the audit is out of date")
	require.False(t, has(api.NetworkServiceParams{}),
		"the network service is gated now; the audit is out of date")
	require.False(t, has(api.EventServiceParams{}),
		"the event service is gated now; the audit is out of date")
	require.False(t, has(api.MetricsServiceParams{}),
		"the metrics service is gated now; the audit is out of date")
	require.False(t, has(api.ProofService{}),
		"the proof service is gated now; the audit is out of date")
}
