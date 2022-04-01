package protocol

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFactoidAddress(t *testing.T) {
	faAddress := "FA2ybgFNYQiZFgTjkwQwp74uGsEUHJc6hGEh4YA3ai7FcssemapP"
	rcdHash, err := GetRCDFromFactoidAddress(faAddress)
	require.NoError(t, err)
	u, err := LiteTokenAddress(rcdHash, ACME)
	require.NoError(t, err)
	t.Logf("FACTOID LITE ACCOUNT ADDRESS FROM FACTOID ADDRESS: %s", u.String())

	u2, err := GetLiteAccountFromFactoidAddress(faAddress)
	require.NoError(t, err)
	require.Equal(t, u.String(), u2.String())
}

func TestRCD(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	rcdHash := GetRCDHashFromPublicKey(pub, 0x01)

	u, err := LiteTokenAddress(rcdHash, ACME)
	require.NoError(t, err)
	t.Logf("FACTOID LITE ACCOUNT ADDRESS FROM PUBLIC KEY: %s", u.String())
}

func TestGetFactoidAddressRcdHashformFactoidPrivate(t *testing.T) {
	FA := "FA2PdKfzGP5XwoSbeW1k9QunCHwC8DY6d8xgEdfm57qfR31nTueb"
	Fs := "Fs1ipNRjEXcWj8RUn1GRLMJYVoPFBL1yw9rn6sCxWGcxciC4HdPd"
	hash1, err := GetRCDFromFactoidAddress(FA)
	require.NoError(t, err)
	add, err := GetFactoidAddressFromRCDHash(hash1[:])
	require.NoError(t, err)
	require.Equal(t, add, FA)
	fa, rcd, _, err := GetFactoidAddressRcdHashPkeyFromPrivateFs(Fs)
	require.Equal(t, hash1, rcd)
	require.NoError(t, err)
	require.Equal(t, FA, fa)
}

func TestFactoidSecretFromPrivKey(t *testing.T) {
	Fs := "Fs1ipNRjEXcWj8RUn1GRLMJYVoPFBL1yw9rn6sCxWGcxciC4HdPd"
	_, _, pk, err := GetFactoidAddressRcdHashPkeyFromPrivateFs(Fs)
	require.NoError(t, err)
	fs, err := GetFactoidSecretFromPrivKey(pk)
	require.NoError(t, err)
	require.Equal(t, fs, Fs)

}

func TestRcdHashAddressFromPrivKey(t *testing.T) {
	FA := "FA2PdKfzGP5XwoSbeW1k9QunCHwC8DY6d8xgEdfm57qfR31nTueb"
	Fs := "Fs1ipNRjEXcWj8RUn1GRLMJYVoPFBL1yw9rn6sCxWGcxciC4HdPd"
	rcd1, err := GetRCDFromFactoidAddress(FA)
	require.NoError(t, err)
	_, rcd2, pk, err := GetFactoidAddressRcdHashPkeyFromPrivateFs(Fs)
	rcd3 := GetRCDHashFromPublicKey(pk[32:], 0x1)
	require.NoError(t, err)
	require.Equal(t, rcd1, rcd2, rcd3)
	add1, _ := GetFactoidAddressFromRCDHash(rcd1)
	add2, _ := GetFactoidAddressFromRCDHash(rcd2)
	add3, _ := GetFactoidAddressFromRCDHash(rcd3)
	require.Equal(t, add1, add2, add3)
./accumulate key import factoid Fs1ipNRjEXcWj8RUn1GRLMJYVoPFBL1yw9rn6sCxWGcxciC4HdPd FA2PdKfzGP5XwoSbeW1k9QunCHwC8DY6d8xgEdfm57qfR31nTueb 

}
