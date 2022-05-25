package e2e

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func send(sim *simulator.Simulator, fn func(func(*Envelope))) []*Envelope {
	sim.Helper()
	var envelopes []*Envelope
	fn(func(tx *Envelope) { envelopes = append(envelopes, tx) })
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(envelopes...)...)
	return envelopes
}

func requireHasKeyHash(t *testing.T, page *protocol.KeyPage, hash []byte) {
	t.Helper()
	_, _, ok := page.EntryByKeyHash(hash)
	require.Truef(t, ok, "Page %v does not contain key hash %x", page.Url, hash[:4])
}

func requireNotHasKeyHash(t *testing.T, page *protocol.KeyPage, hash []byte) {
	t.Helper()
	_, _, ok := page.EntryByKeyHash(hash)
	require.Falsef(t, ok, "Page %v does not contain key hash %x", page.Url, hash[:4])
}

func TestUpdateValidators(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	dn := sim.Subnet(Directory)
	validators := FormatKeyPageUrl(dn.Executor.Network.ValidatorBook(), 0)
	nodeKeyAdd1, nodeKeyAdd2, nodeKeyAdd3, nodeKeyUpd := acctesting.GenerateKey(1), acctesting.GenerateKey(2), acctesting.GenerateKey(3), acctesting.GenerateKey(4)

	// The validator timestamp starts out > 0
	signer := simulator.GetAccount[*KeyPage](sim, dn.Executor.Network.DefaultValidatorPage())
	_, entry, ok := signer.EntryByKey(dn.Executor.Key[32:])
	require.True(t, ok)
	timestamp = entry.GetLastUsedOn()

	// Update NetworkGlobals - use 5/12 so that M = 1 for 3 validators and M = 2
	// for 4
	ng := new(NetworkGlobals)
	ng.ValidatorThreshold.Set(5, 12)
	wd := new(WriteData)
	d, err := ng.MarshalBinary()
	require.NoError(t, err)
	wd.Entry = &AccumulateDataEntry{Data: [][]byte{d}}
	send(sim,
		func(send func(*Envelope)) {
			send(acctesting.NewTransaction().
				WithPrincipal(dn.Executor.Network.NodeUrl(Globals)).
				WithTimestampVar(&timestamp).
				WithSigner(dn.Executor.Network.ValidatorPage(0), 1). // TODO move back to OperatorPage in or after AC-1402
				WithBody(wd).
				Initiate(SignatureTypeLegacyED25519, dn.Executor.Key).
				Build())
		})

	// Verify there is one validator (node key)
	require.ElementsMatch(t, dn.Validators, [][]byte{dn.Executor.Key[32:]})

	// Add a validator
	send(sim,
		func(send func(*Envelope)) {
			body := new(AddValidator)
			body.PubKey = nodeKeyAdd1[32:]
			send(acctesting.NewTransaction().
				WithPrincipal(dn.Executor.Network.NodeUrl(ValidatorBook)).
				WithTimestampVar(&timestamp).
				WithSigner(validators, 1).
				WithBody(body).
				Initiate(SignatureTypeLegacyED25519, dn.Executor.Key).
				Build())
		})

	// Verify the validator was added
	require.ElementsMatch(t, dn.Validators, [][]byte{dn.Executor.Key[32:], nodeKeyAdd1[32:]})

	// Update a validator
	send(sim,
		func(send func(*Envelope)) {
			body := new(UpdateValidatorKey)

			body.PubKey = nodeKeyAdd1[32:]
			body.NewPubKey = nodeKeyUpd[32:]

			send(acctesting.NewTransaction().
				WithPrincipal(dn.Executor.Network.NodeUrl(ValidatorBook)).
				WithTimestampVar(&timestamp).
				WithSigner(validators, 2).
				WithBody(body).
				Initiate(SignatureTypeLegacyED25519, dn.Executor.Key).
				Build())
		})

	// Verify the validator was updated
	require.ElementsMatch(t, dn.Validators, [][]byte{dn.Executor.Key[32:], nodeKeyUpd[32:]})

	// Add a third validator
	send(sim,
		func(send func(*Envelope)) {
			body := new(AddValidator)
			body.PubKey = nodeKeyAdd2[32:]
			send(acctesting.NewTransaction().
				WithPrincipal(dn.Executor.Network.NodeUrl(ValidatorBook)).
				WithTimestampVar(&timestamp).
				WithSigner(validators, 3).
				WithBody(body).
				Initiate(SignatureTypeLegacyED25519, dn.Executor.Key).
				Build())
		})

	// Verify the validator was added
	require.ElementsMatch(t, dn.Validators, [][]byte{dn.Executor.Key[32:], nodeKeyUpd[32:], nodeKeyAdd2[32:]})

	// Verify the Validator threshold
	require.Equal(t, uint64(1), simulator.GetAccount[*KeyPage](sim, validators).AcceptThreshold)

	// Add a fourth validator, so the page threshold will become 2
	send(sim,
		func(send func(*Envelope)) {
			body := new(AddValidator)
			body.PubKey = nodeKeyAdd3[32:]

			send(acctesting.NewTransaction().
				WithPrincipal(dn.Executor.Network.NodeUrl(ValidatorBook)).
				WithTimestampVar(&timestamp).
				WithSigner(validators, 4).
				WithBody(body).
				Initiate(SignatureTypeLegacyED25519, dn.Executor.Key).
				Build())
		})

	// Verify the validator was added
	require.ElementsMatch(t, dn.Validators, [][]byte{dn.Executor.Key[32:], nodeKeyUpd[32:], nodeKeyAdd2[32:], nodeKeyAdd3[32:]})

	// Verify the Validator threshold
	require.Equal(t, uint64(2), simulator.GetAccount[*KeyPage](sim, validators).AcceptThreshold)

	// Remove a validator
	send(sim,
		func(send func(*Envelope)) {
			body := new(RemoveValidator)
			body.PubKey = nodeKeyUpd[32:]

			send(acctesting.NewTransaction().
				WithPrincipal(dn.Executor.Network.NodeUrl(ValidatorBook)).
				WithTimestampVar(&timestamp).
				WithSigner(validators, 5).
				WithBody(body).
				Initiate(SignatureTypeLegacyED25519, dn.Executor.Key).
				Sign(SignatureTypeED25519, nodeKeyAdd2).
				Build())
		})

	// Verify the validator was removed
	require.ElementsMatch(t, dn.Validators, [][]byte{dn.Executor.Key[32:], nodeKeyAdd2[32:], nodeKeyAdd3[32:]})
}

func TestUpdateOperators(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()
	dn := sim.Subnet(protocol.Directory)
	bvn0 := sim.Subnet(sim.Subnets[1].ID)
	bvn1 := sim.Subnet(sim.Subnets[2].ID)

	// Sanity check
	page := simulator.GetAccount[*KeyPage](sim, bvn0.Executor.Network.DefaultOperatorPage())
	require.Len(t, page.Keys, 4)

	// Add
	t.Log("Add")
	operators := dn.Executor.Network.DefaultOperatorPage()
	opKeyAdd := acctesting.GenerateKey(1)
	addKeyHash := sha256.Sum256(opKeyAdd[32:])
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(operators).
			WithTimestampVar(&timestamp).
			WithSigner(operators, 1).
			WithBody(&UpdateKeyPage{Operation: []KeyPageOperation{
				&AddKeyOperation{
					Entry: KeySpecParams{KeyHash: addKeyHash[:]},
				},
			}}).
			Initiate(SignatureTypeED25519, dn.Executor.Key).
			Sign(SignatureTypeED25519, bvn0.Executor.Key).
			Sign(SignatureTypeED25519, bvn1.Executor.Key).
			Build(),
	)...)

	// Give it a second for the DN to send its anchor
	sim.ExecuteBlocks(10)

	// Verify the update was pushed
	page = simulator.GetAccount[*KeyPage](sim, bvn0.Executor.Network.DefaultOperatorPage())
	require.Len(t, page.Keys, 5)
	requireHasKeyHash(t, page, addKeyHash[:])

	// Update
	t.Log("Update")
	opKeyUpd := acctesting.GenerateKey(2)
	updKeyHash := sha256.Sum256(opKeyUpd[32:])
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(operators).
			WithTimestampVar(&timestamp).
			WithSigner(operators, 2).
			WithBody(&UpdateKeyPage{Operation: []KeyPageOperation{
				&UpdateKeyOperation{
					OldEntry: KeySpecParams{KeyHash: addKeyHash[:]},
					NewEntry: KeySpecParams{KeyHash: updKeyHash[:]},
				},
			}}).
			Initiate(SignatureTypeED25519, dn.Executor.Key).
			Sign(SignatureTypeED25519, bvn0.Executor.Key).
			Sign(SignatureTypeED25519, bvn1.Executor.Key).
			Build(),
	)...)

	// Give it a second for the DN to send its anchor
	sim.ExecuteBlocks(10)

	// Verify the update was pushed
	page = simulator.GetAccount[*KeyPage](sim, bvn0.Executor.Network.DefaultOperatorPage())
	require.Len(t, page.Keys, 5)
	requireNotHasKeyHash(t, page, addKeyHash[:])
	requireHasKeyHash(t, page, updKeyHash[:])

	// Remove
	t.Log("Remove")
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(operators).
			WithTimestampVar(&timestamp).
			WithSigner(operators, 3).
			WithBody(&UpdateKeyPage{Operation: []KeyPageOperation{
				&RemoveKeyOperation{
					Entry: KeySpecParams{KeyHash: updKeyHash[:]},
				},
			}}).
			Initiate(SignatureTypeED25519, dn.Executor.Key).
			Sign(SignatureTypeED25519, bvn0.Executor.Key).
			Sign(SignatureTypeED25519, bvn1.Executor.Key).
			Build(),
	)...)

	// Give it a second for the DN to send its anchor
	sim.ExecuteBlocks(10)

	// Verify the update was pushed
	page = simulator.GetAccount[*KeyPage](sim, bvn0.Executor.Network.DefaultOperatorPage())
	require.Len(t, page.Keys, 4)
	requireNotHasKeyHash(t, page, updKeyHash[:])
}
