// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol_test

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type FactoidPair struct {
	Fs   string
	FA   string
	Lite string
}

var TestFct = []FactoidPair{
	{"Fs1jQGc9GJjyWNroLPq7x6LbYQHveyjWNPXSqAvCEKpETNoTU5dP", "FA22de5NSG2FA2HmMaD4h8qSAZAJyztmmnwgLPghCQKoSekwYYct", "acc://08115f96ebb5e35a9c806de9cffe4c99455a0c5a60942d53/ACME"},
	{"Fs2wZzM2iBn4HEbhwEUZjLfcbTo5Rf6ChRNjNJWDiyWmy9zkPQNP", "FA3heCmxKCk1tCCfiAMDmX8Ctg6XTQjRRaJrF5Jagc9rbo7wqQLV", "acc://e4571e13d3af400ad41a7e70134387d0f9b0bd5a94f4347f/ACME"},
	{"Fs1fxJbUWQRbTXH4as6qazoZ3hunmzL9JfiEpA6diCGCBE4jauqs", "FA2PSjogJ7UWwrwtevXtoRDnpxeafuRno16pES7KY4i51pL3kWV5", "acc://3752fc879ff3538e4e436512191aec2b61f8a9374c38f723/ACME"},
	{"Fs2Fb4aB1N8VCBkQmeYMnLSNFtwGeRkqSmDbgw5BLa2Gdd8SVXjh", "FA3BW5bhnR6vBba42djAL9QMuKPafJurmGxM499u43PNFUdvKEKz", "acc://9fe752486d3f03a607b465c0766947f86a8242de54e0c0c4/ACME"},
	{"Fs34u8hHboYaeisKpjt8AaGDr97zSviP5n5KmzD8FteSjjvSNA7D", "FA3sMQeEgh2z6Hr5Pr8Kfnhh49QchVpmitGswUJjc1Mw3B3BW727", "acc://fa639612f2567f4ceb92a516c3dacc715d349575dbdc81d8/ACME"},
}

func TestFactoidAddress(t *testing.T) {
	faAddress := "FA2ybgFNYQiZFgTjkwQwp74uGsEUHJc6hGEh4YA3ai7FcssemapP"
	rcdHash, err := protocol.GetRCDFromFactoidAddress(faAddress)
	require.NoError(t, err)
	u, err := protocol.LiteTokenAddressFromHash(rcdHash, protocol.ACME)
	require.NoError(t, err)
	t.Logf("FACTOID LITE ACCOUNT ADDRESS FROM FACTOID ADDRESS: %s", u.String())

	u2, err := protocol.GetLiteAccountFromFactoidAddress(faAddress)
	require.NoError(t, err)
	require.Equal(t, u.String(), u2.String())
}

func TestRCD(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	rcdHash := protocol.GetRCDHashFromPublicKey(pub, 0x01)

	u, err := protocol.LiteTokenAddressFromHash(rcdHash, protocol.ACME)
	require.NoError(t, err)

	u2, err := protocol.LiteTokenAddress(pub, protocol.ACME, protocol.SignatureTypeRCD1)
	require.NoError(t, err)
	require.Equal(t, u.String(), u2.String())
	t.Logf("FACTOID LITE ACCOUNT ADDRESS FROM PUBLIC KEY: %s", u.String())
}

func TestGetFactoidAddressRcdHashformFactoidPrivate(t *testing.T) {
	FA := "FA2PdKfzGP5XwoSbeW1k9QunCHwC8DY6d8xgEdfm57qfR31nTueb"
	Fs := "Fs1ipNRjEXcWj8RUn1GRLMJYVoPFBL1yw9rn6sCxWGcxciC4HdPd"
	hash1, err := protocol.GetRCDFromFactoidAddress(FA)
	require.NoError(t, err)
	add, err := protocol.GetFactoidAddressFromRCDHash(hash1[:])
	require.NoError(t, err)
	require.Equal(t, add, FA)
	fa, rcd, _, err := protocol.GetFactoidAddressRcdHashPkeyFromPrivateFs(Fs)
	require.Equal(t, hash1, rcd)
	require.NoError(t, err)
	require.Equal(t, FA, fa)
}

func TestFactoidSecretFromPrivKey(t *testing.T) {
	Fs := "Fs1ipNRjEXcWj8RUn1GRLMJYVoPFBL1yw9rn6sCxWGcxciC4HdPd"
	_, _, pk, err := protocol.GetFactoidAddressRcdHashPkeyFromPrivateFs(Fs)
	require.NoError(t, err)
	fs, err := protocol.GetFactoidSecretFromPrivKey(pk)
	require.NoError(t, err)
	require.Equal(t, fs, Fs)

}

func TestRcdHashAddressFromPrivKey(t *testing.T) {
	for _, addr := range TestFct {
		rcd1, err := protocol.GetRCDFromFactoidAddress(addr.FA)
		require.NoError(t, err)
		faTst, rcd2, pk, err := protocol.GetFactoidAddressRcdHashPkeyFromPrivateFs(addr.Fs)
		require.Equal(t, addr.FA, faTst)
		rcd3 := protocol.GetRCDHashFromPublicKey(pk[32:], 0x1)
		require.NoError(t, err)
		require.Equal(t, rcd1, rcd2, rcd3)
		add1, _ := protocol.GetFactoidAddressFromRCDHash(rcd1)
		add2, _ := protocol.GetFactoidAddressFromRCDHash(rcd2)
		add3, _ := protocol.GetFactoidAddressFromRCDHash(rcd3)
		require.Equal(t, add1, add2, add3)
		u, err := protocol.LiteTokenAddressFromHash(rcd1, protocol.ACME)
		require.NoError(t, err)
		require.Equal(t, addr.Lite, u.String())
	}
}
