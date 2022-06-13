package e2e

import (
	"crypto/ed25519"
	"crypto/sha256"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func send(sim *simulator.Simulator, fn func(func(*Envelope))) []*Envelope {
	sim.Helper()
	var envelopes []*Envelope
	fn(func(tx *Envelope) { envelopes = append(envelopes, tx) })
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(envelopes...)...)
	return envelopes
}

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
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	dn := sim.Subnet(Directory)
	bvn0 := sim.Subnet("BVN0")
	bvn1 := sim.Subnet("BVN1")
	signerUrl := dn.Executor.Describe.DefaultOperatorPage()
	vldKey1, vldKey2, vldKey3, vldKey4 := acctesting.GenerateKey(1), acctesting.GenerateKey(2), acctesting.GenerateKey(3), acctesting.GenerateKey(4)
	height := uint64(1)

	// The validator timestamp starts out > 0
	signer := simulator.GetAccount[*KeyPage](sim, signerUrl)
	_, entry, ok := signer.EntryByKey(dn.Executor.Key[32:])
	require.True(t, ok)
	timestamp = entry.GetLastUsedOn()

	// Update NetworkGlobals - use 5/12 so that M = 1 for 3 validators and M = 2
	// for 4
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(5, 12)
	send(sim,
		func(send func(*Envelope)) {
			send(acctesting.NewTransaction().
				WithPrincipal(dn.Executor.Describe.NodeUrl(Globals)).
				WithTimestampVar(&timestamp).
				WithSigner(signerUrl, 1).
				WithBody(&WriteData{
					Entry:        g.FormatGlobals(),
					WriteToState: true,
				}).
				Initiate(SignatureTypeLegacyED25519, dn.Executor.Key).
				Sign(SignatureTypeED25519, bvn0.Executor.Key).
				Sign(SignatureTypeED25519, bvn1.Executor.Key).
				Build())
		})

	// Verify there is one validator (node key)
	require.ElementsMatch(t, dn.Validators, [][]byte{dn.Executor.Key[32:]})

	// Add a validator
	addOperatorKey(t, sim, dn, vldKey1, &timestamp, &height, bvn0.Executor.Key, bvn1.Executor.Key)
	send(sim,
		func(send func(*Envelope)) {
			body := new(AddValidator)
			body.PubKey = vldKey1.Public().(ed25519.PublicKey)
			send(acctesting.NewTransaction().
				WithPrincipal(dn.Executor.Describe.NodeUrl(ValidatorBook)).
				WithTimestampVar(&timestamp).
				WithSigner(signerUrl, height).
				WithBody(body).
				Initiate(SignatureTypeLegacyED25519, dn.Executor.Key).
				Sign(SignatureTypeED25519, bvn0.Executor.Key).
				Sign(SignatureTypeED25519, bvn1.Executor.Key).
				Build())
		})

	// Verify the validator was added
	require.ElementsMatch(t, dn.Validators, [][]byte{dn.Executor.Key[32:], vldKey1[32:]})

	// Update a validator
	send(sim,
		func(send func(*Envelope)) {
			body := new(UpdateValidatorKey)

			body.PubKey = vldKey1[32:]
			body.NewPubKey = vldKey4[32:]

			send(acctesting.NewTransaction().
				WithPrincipal(dn.Executor.Describe.NodeUrl(ValidatorBook)).
				WithTimestampVar(&timestamp).
				WithSigner(signerUrl, height).
				WithBody(body).
				Initiate(SignatureTypeLegacyED25519, dn.Executor.Key).
				Sign(SignatureTypeED25519, bvn0.Executor.Key).
				Sign(SignatureTypeED25519, bvn1.Executor.Key).
				Build())
		})

	// Verify the validator was updated
	require.ElementsMatch(t, dn.Validators, [][]byte{dn.Executor.Key[32:], vldKey4[32:]})

	// Add a third validator, so the page threshold will become 3
	addOperatorKey(t, sim, dn, vldKey2, &timestamp, &height, bvn0.Executor.Key, bvn1.Executor.Key)
	send(sim,
		func(send func(*Envelope)) {
			body := new(AddValidator)
			body.PubKey = vldKey2[32:]
			send(acctesting.NewTransaction().
				WithPrincipal(dn.Executor.Describe.NodeUrl(ValidatorBook)).
				WithTimestampVar(&timestamp).
				WithSigner(signerUrl, height).
				WithBody(body).
				Initiate(SignatureTypeLegacyED25519, dn.Executor.Key).
				Sign(SignatureTypeED25519, bvn0.Executor.Key).
				Sign(SignatureTypeED25519, bvn1.Executor.Key).
				Build())
		})

	// Verify the validator was added
	require.ElementsMatch(t, dn.Validators, [][]byte{dn.Executor.Key[32:], vldKey4[32:], vldKey2[32:]})

	// Verify the Validator threshold
	oprPage := simulator.GetAccount[*KeyPage](sim, signerUrl)
	require.Equal(t, uint64(3), oprPage.AcceptThreshold)

	// Add a fourth validator
	addOperatorKey(t, sim, dn, vldKey3, &timestamp, &height, bvn0.Executor.Key, bvn1.Executor.Key)
	send(sim,
		func(send func(*Envelope)) {
			body := new(AddValidator)
			body.PubKey = vldKey3[32:]

			send(acctesting.NewTransaction().
				WithPrincipal(dn.Executor.Describe.NodeUrl(ValidatorBook)).
				WithTimestampVar(&timestamp).
				WithSigner(signerUrl, height).
				WithBody(body).
				Initiate(SignatureTypeLegacyED25519, dn.Executor.Key).
				Sign(SignatureTypeED25519, bvn0.Executor.Key).
				Sign(SignatureTypeED25519, bvn1.Executor.Key).
				Build())
		})

	// Verify the validator was added
	require.ElementsMatch(t, dn.Validators, [][]byte{dn.Executor.Key[32:], vldKey4[32:], vldKey2[32:], vldKey3[32:]})

	// Verify the Validator threshold
	require.Equal(t, uint64(3), simulator.GetAccount[*KeyPage](sim, signerUrl).AcceptThreshold)

	// Remove a validator
	send(sim,
		func(send func(*Envelope)) {
			body := new(RemoveValidator)
			body.PubKey = vldKey4[32:]

			send(acctesting.NewTransaction().
				WithPrincipal(dn.Executor.Describe.NodeUrl(ValidatorBook)).
				WithTimestampVar(&timestamp).
				WithSigner(signerUrl, height).
				WithBody(body).
				Initiate(SignatureTypeLegacyED25519, dn.Executor.Key).
				Sign(SignatureTypeED25519, bvn0.Executor.Key).
				Sign(SignatureTypeED25519, vldKey1).
				Build())
		})

	// Verify the validator was removed
	require.ElementsMatch(t, dn.Validators, [][]byte{dn.Executor.Key[32:], vldKey2[32:], vldKey3[32:]})
}

func addOperatorKey(t *testing.T, sim *simulator.Simulator, dn *simulator.ExecEntry, oprKey ed25519.PrivateKey, timestamp *uint64, height *uint64, signKeys ...ed25519.PrivateKey) {

	// See if we are going to need to sign and have enough keys
	oprPage := simulator.GetAccount[*KeyPage](sim, dn.Executor.Describe.DefaultOperatorPage())
	threshold := oprPage.AcceptThreshold
	nrKeys := uint64(len(signKeys))
	require.Equalf(t, threshold-1, nrKeys, "we need %d extra signing keys but have %d", threshold-1, nrKeys)

	send(sim,
		func(send func(*Envelope)) {
			op := new(AddKeyOperation)
			pubKey := oprKey.Public().(ed25519.PublicKey)
			keyHash := sha256.Sum256(pubKey)
			op.Entry.KeyHash = keyHash[:]
			body := new(UpdateKeyPage)
			body.Operation = append(body.Operation, op)
			txb := acctesting.NewTransaction().
				WithPrincipal(dn.Executor.Describe.DefaultOperatorPage()).
				WithTimestampVar(timestamp).
				WithSigner(dn.Executor.Describe.DefaultOperatorPage(), *height).
				WithBody(body).
				Initiate(SignatureTypeLegacyED25519, dn.Executor.Key)
			for _, signKey := range signKeys {
				txb = txb.Sign(SignatureTypeED25519, signKey)
			}
			send(txb.Build())
		})
	atomic.AddUint64(height, 1)
}

func TestUpdateOperators(t *testing.T) {
	var timestamp uint64

	// Initialize
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	sim := simulator.New(t, 3)
	sim.InitFromGenesisWith(g)
	dn := sim.Subnet(Directory)
	bvn0 := sim.Subnet(sim.Subnets[1].Id)
	bvn1 := sim.Subnet(sim.Subnets[2].Id)
	bvn2 := sim.Subnet(sim.Subnets[3].Id)

	// Sanity check
	page := simulator.GetAccount[*KeyPage](sim, bvn0.Executor.Describe.DefaultOperatorPage())
	require.Len(t, page.Keys, 4)

	// Add
	t.Log("Add")
	signerUrl := dn.Executor.Describe.DefaultOperatorPage()
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
			Sign(SignatureTypeED25519, bvn0.Executor.Key).
			Sign(SignatureTypeED25519, bvn1.Executor.Key).
			Sign(SignatureTypeED25519, bvn2.Executor.Key).
			Build(),
	)...)

	// Give it a second for the DN to send its anchor
	sim.ExecuteBlocks(10)

	// Verify the update was pushed
	page = simulator.GetAccount[*KeyPage](sim, bvn0.Executor.Describe.DefaultOperatorPage())
	require.Len(t, page.Keys, 5)
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
			Sign(SignatureTypeED25519, bvn0.Executor.Key).
			Sign(SignatureTypeED25519, bvn1.Executor.Key).
			Sign(SignatureTypeED25519, bvn2.Executor.Key).
			Build(),
	)...)

	// Give it a second for the DN to send its anchor
	sim.ExecuteBlocks(10)

	// Verify the update was pushed
	page = simulator.GetAccount[*KeyPage](sim, bvn0.Executor.Describe.DefaultOperatorPage())
	require.Len(t, page.Keys, 5)
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
			Sign(SignatureTypeED25519, bvn0.Executor.Key).
			Sign(SignatureTypeED25519, bvn1.Executor.Key).
			Sign(SignatureTypeED25519, bvn2.Executor.Key).
			Build(),
	)...)

	// Give it a second for the DN to send its anchor
	sim.ExecuteBlocks(10)

	// Verify the update was pushed
	page = simulator.GetAccount[*KeyPage](sim, bvn0.Executor.Describe.DefaultOperatorPage())
	require.Len(t, page.Keys, 4)
	requireNotHasKeyHash(t, page, updKeyHash[:])
}
