// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/v1/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func requireHasKeyHash(t *testing.T, page *KeyPage, hash []byte) {
	t.Helper()
	_, _, ok := page.EntryByKeyHash(hash)
	require.Truef(t, ok, "Page %v does not contain key hash %x", page.Url, hash[:4])
}

func requireNotHasKeyHash(t *testing.T, page *KeyPage, hash []byte) {
	t.Helper()
	_, _, ok := page.EntryByKeyHash(hash)
	require.Falsef(t, ok, "Page %v does not contain key hash %x", page.Url, hash[:4])
}

func TestUpdateValidators(t *testing.T) {
	var timestamp uint64

	// Initialize
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 3) // Use 1/3 so that M = 1 for 3 validators and M = 2 for 4
	sim := simulator.New(t, 3)
	sim.InitFromGenesisWith(g)

	dn := sim.Partition(Directory)
	initVal := make([][]byte, len(dn.Validators))
	copy(initVal, dn.Validators)
	operators := dn.Executor.Describe.OperatorsPage()
	vldKey1, vldKey2, vldKey3, vldKey4 := acctesting.GenerateKey(1), acctesting.GenerateKey(2), acctesting.GenerateKey(3), acctesting.GenerateKey(4)
	vldKey1Hash, vldKey2Hash, vldKey3Hash, vldKey4Hash := sha256.Sum256(vldKey1[32:]), sha256.Sum256(vldKey2[32:]), sha256.Sum256(vldKey3[32:]), sha256.Sum256(vldKey4[32:])

	// The validator timestamp starts out > 0
	page := simulator.GetAccount[*KeyPage](sim, operators)
	_, entry, ok := page.EntryByKey(dn.Executor.Key[32:])
	require.True(t, ok)
	timestamp = entry.GetLastUsedOn()
	signer1 := new(signing.Builder).
		SetType(SignatureTypeED25519).
		UseSimpleHash().
		SetPrivateKey(dn.Executor.Key).
		SetUrl(operators).
		SetVersion(page.Version).
		SetTimestampWithVar(&timestamp)

	// Additional signers
	signer2 := new(signing.Builder).
		SetType(SignatureTypeED25519).
		UseSimpleHash().
		SetPrivateKey(sim.Partition("BVN0").Executor.Key).
		SetUrl(operators).
		SetVersion(page.Version)
	signer3 := new(signing.Builder).
		SetType(SignatureTypeED25519).
		UseSimpleHash().
		SetPrivateKey(sim.Partition("BVN1").Executor.Key).
		SetUrl(operators).
		SetVersion(page.Version)

	// Verify the threshold starts at 1
	require.Equal(t, 1, int(page.AcceptThreshold))

	// Add a validator
	values := dn.Executor.ActiveGlobals_TESTONLY()
	envs, err := AddOperator(values, len(page.Keys), vldKey1[32:], vldKey1Hash[:], Directory, signer1, signer2, signer3)
	require.NoError(t, err)
	for _, env := range envs {
		sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(env)...)
	}

	// Verify the validator was added
	require.ElementsMatch(t, dn.Validators, append(initVal, vldKey1[32:]))

	// Verify the threshold increased to 2
	page = simulator.GetAccount[*KeyPage](sim, operators)
	require.Equal(t, 2, int(page.AcceptThreshold))

	// Update a validator
	signer1.SetVersion(page.Version)

	values = dn.Executor.ActiveGlobals_TESTONLY()
	envs, err = UpdateOperatorKey(values, vldKey1[32:], vldKey1Hash[:], vldKey4[32:], vldKey4Hash[:], signer1, signer2, signer3)
	require.NoError(t, err)
	for _, env := range envs {
		sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(env)...)
	}

	// Verify the validator was updated
	require.ElementsMatch(t, dn.Validators, append(initVal, vldKey4[32:]))

	// Add a third validator, so the page threshold will become 2
	page = simulator.GetAccount[*KeyPage](sim, operators)
	signer1.SetVersion(page.Version)

	values = dn.Executor.ActiveGlobals_TESTONLY()
	envs, err = AddOperator(values, len(page.Keys), vldKey2[32:], vldKey2Hash[:], Directory, signer1, signer2, signer3)
	require.NoError(t, err)
	for _, env := range envs {
		sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(env)...)
	}

	// Verify the validator was added
	require.ElementsMatch(t, dn.Validators, append(initVal, vldKey4[32:], vldKey2[32:]))

	// Add a fourth validator
	page = simulator.GetAccount[*KeyPage](sim, operators)
	signer1.SetVersion(page.Version)
	signerA2 := signer1.Copy().SetPrivateKey(vldKey2).ClearTimestamp()

	values = dn.Executor.ActiveGlobals_TESTONLY()
	envs, err = AddOperator(values, len(page.Keys), vldKey3[32:], vldKey3Hash[:], Directory, signer1, signer2, signer3, signerA2)
	require.NoError(t, err)
	for _, env := range envs {
		sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(env)...)
	}

	// Verify the validator was added
	require.ElementsMatch(t, dn.Validators, append(initVal, vldKey4[32:], vldKey2[32:], vldKey3[32:]))

	// Remove a validator
	page = simulator.GetAccount[*KeyPage](sim, operators)
	signer1.SetVersion(page.Version)

	values = dn.Executor.ActiveGlobals_TESTONLY()
	envs, err = RemoveOperator(values, len(page.Keys), vldKey4[32:], vldKey4Hash[:], signer1, signer2, signer3, signerA2)
	require.NoError(t, err)
	for _, env := range envs {
		sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(env)...)
	}

	// Verify the validator was removed
	require.ElementsMatch(t, dn.Validators, append(initVal, vldKey2[32:], vldKey3[32:]))
}

func TestUpdateOperators(t *testing.T) {
	var timestamp uint64

	// Initialize
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	sim := simulator.New(t, 3)
	sim.InitFromGenesisWith(g)
	dn := sim.Partition(Directory)
	bvn0 := sim.Partition("BVN0")

	page := simulator.GetAccount[*KeyPage](sim, bvn0.Executor.Describe.OperatorsPage())
	initValCount := len(page.Keys)

	// Add
	t.Log("Add")
	signerUrl := dn.Executor.Describe.OperatorsPage()
	opKeyAdd := acctesting.GenerateKey(1)
	addKeyHash := sha256.Sum256(opKeyAdd[32:])
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(signerUrl).
			WithTimestampVar(&timestamp).
			WithSigner(signerUrl, 1).
			WithBody(&UpdateKeyPage{Operation: []KeyPageOperation{
				&AddKeyOperation{
					Entry: KeySpecParams{KeyHash: addKeyHash[:]},
				},
			}}).
			Initiate(SignatureTypeED25519, dn.Executor.Key).
			Build(),
	)...)

	// Give it a second for the DN to send its anchor
	sim.ExecuteBlocks(10)

	// Verify the update was pushed
	page = simulator.GetAccount[*KeyPage](sim, bvn0.Executor.Describe.OperatorsPage())
	require.Len(t, page.Keys, initValCount+1)
	requireHasKeyHash(t, page, addKeyHash[:])

	// Update
	t.Log("Update")
	opKeyUpd := acctesting.GenerateKey(2)
	updKeyHash := sha256.Sum256(opKeyUpd[32:])
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(signerUrl).
			WithTimestampVar(&timestamp).
			WithSigner(signerUrl, 2).
			WithBody(&UpdateKeyPage{Operation: []KeyPageOperation{
				&UpdateKeyOperation{
					OldEntry: KeySpecParams{KeyHash: addKeyHash[:]},
					NewEntry: KeySpecParams{KeyHash: updKeyHash[:]},
				},
			}}).
			Initiate(SignatureTypeED25519, dn.Executor.Key).
			Build(),
	)...)

	// Give it a second for the DN to send its anchor
	sim.ExecuteBlocks(10)

	// Verify the update was pushed
	page = simulator.GetAccount[*KeyPage](sim, bvn0.Executor.Describe.OperatorsPage())
	require.Len(t, page.Keys, initValCount+1)
	requireNotHasKeyHash(t, page, addKeyHash[:])
	requireHasKeyHash(t, page, updKeyHash[:])

	// Remove
	t.Log("Remove")
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(signerUrl).
			WithTimestampVar(&timestamp).
			WithSigner(signerUrl, 3).
			WithBody(&UpdateKeyPage{Operation: []KeyPageOperation{
				&RemoveKeyOperation{
					Entry: KeySpecParams{KeyHash: updKeyHash[:]},
				},
			}}).
			Initiate(SignatureTypeED25519, dn.Executor.Key).
			Build(),
	)...)

	// Give it a second for the DN to send its anchor
	sim.ExecuteBlocks(10)

	// Verify the update was pushed
	page = simulator.GetAccount[*KeyPage](sim, bvn0.Executor.Describe.OperatorsPage())
	require.Len(t, page.Keys, initValCount)
	requireNotHasKeyHash(t, page, updKeyHash[:])
}
