// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4148: this line gates SetLiteAccountDelegate on Tanegashima; main runs it
// ungated. These tests pin the gate on the line that HAS one, so whichever
// way the decision goes, changing the gate is a deliberate edit here.

func sladStateManager(t *testing.T, version protocol.ExecutorVersion, origin protocol.Account, body *protocol.SetLiteAccountDelegate) (*StateManager, *Delivery) {
	t.Helper()
	txn := new(protocol.Transaction)
	txn.Header.Principal = origin.GetUrl()
	txn.Body = body

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	globals := &core.GlobalValues{ExecutorVersion: version}
	shim := execute.DescribeShim{NetworkType: protocol.PartitionTypeBlockValidator, PartitionId: "BVN0"}
	st := NewStateManager(shim, globals, nil, batch, origin, txn, nil)
	return st, &Delivery{Transaction: txn}
}

func liteOrigin() *protocol.LiteTokenAccount {
	lid := protocol.LiteAuthorityForKey(make([]byte, 32), protocol.SignatureTypeED25519)
	return &protocol.LiteTokenAccount{Url: lid.JoinPath("ACME"), TokenUrl: protocol.AcmeUrl()}
}

func TestSetLiteAccountDelegate_RejectedBelowActivation(t *testing.T) {
	st, tx := sladStateManager(t, protocol.ExecutorVersionV2Jiuquan, liteOrigin(), &protocol.SetLiteAccountDelegate{})

	_, err := SetLiteAccountDelegate{}.Validate(st, tx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errors.NotAllowed), "below the gate the type is not allowed")
	assert.ErrorContains(t, err, "requires protocol version v2-tanegashima",
		"the rejection must name the version that activates the type")
}

func TestSetLiteAccountDelegate_AllowedAtActivationVersion(t *testing.T) {
	st, tx := sladStateManager(t, protocol.ExecutorVersionV2Tanegashima, liteOrigin(), &protocol.SetLiteAccountDelegate{})

	_, err := SetLiteAccountDelegate{}.Validate(st, tx)
	assert.NoError(t, err, "at Tanegashima the gate opens")
}

func TestSetLiteAccountDelegate_AllowedAboveActivationVersion(t *testing.T) {
	for _, v := range []protocol.ExecutorVersion{protocol.ExecutorVersionV2Kourou, protocol.ExecutorVersionVNext} {
		st, tx := sladStateManager(t, v, liteOrigin(), &protocol.SetLiteAccountDelegate{})
		_, err := SetLiteAccountDelegate{}.Validate(st, tx)
		assert.NoError(t, err, "the gate must stay open at %v", v)
	}
}

// Clearing a delegate (body.Delegate == nil) needs no proof — pinned so the
// gate's arrival cannot silently change the clearing path.
func TestSetLiteAccountDelegate_ClearingADelegateNeedsNoProof(t *testing.T) {
	st, tx := sladStateManager(t, protocol.ExecutorVersionV2Tanegashima, liteOrigin(),
		&protocol.SetLiteAccountDelegate{Delegate: nil, Proof: nil})

	_, err := SetLiteAccountDelegate{}.Validate(st, tx)
	assert.NoError(t, err, "clearing delegation requires neither delegate nor proof")
}

// Characterization of a LEAK, not an endorsement: the version gate lives in
// Validate only — Execute never checks it. A transaction that skips CheckTx
// (a loose or malicious leader including it directly in a block) executes
// below the activation version. This test pins the current shape so closing
// the leak — or deciding the gate should come out entirely, per #4148 — is a
// visible change.
func TestSetLiteAccountDelegate_GateAppliesToValidateOnly_ExecuteLeaks(t *testing.T) {
	adi := &protocol.ADI{Url: protocol.AccountUrl("alice")}
	st, tx := sladStateManager(t, protocol.ExecutorVersionV2Jiuquan, adi, &protocol.SetLiteAccountDelegate{})

	_, err := SetLiteAccountDelegate{}.Execute(st, tx)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "requires protocol version",
		"Execute runs PAST the version gate — it fails on the principal type instead")
	assert.ErrorContains(t, err, "requires a lite token account",
		"proof the executor body ran below the gate: it reached the principal check")
}
