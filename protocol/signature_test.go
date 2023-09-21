// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	btc "github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcutil/base58"
	eth "github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() {
	acctesting.EnableDebugFeatures()
}

func TestBTCSignature(t *testing.T) {

	//m/44'/60'/0'/0/0 yellow ->
	privKey := base58.Decode("KxukKhTPU11xH2Wfk2366e375166QE4r7y8FWojU9XPbzLYYSM3j")

	message := "ACME will rule DEFI"
	hash := sha256.Sum256([]byte(message))
	secp := new(BTCSignature)

	privkey, pbkey := btc.PrivKeyFromBytes(btc.S256(), privKey)

	secp.PublicKey = pbkey.SerializeCompressed()

	require.NoError(t, SignBTC(secp, privkey.Serialize(), nil, hash[:]))
	res := secp.Verify(nil, hash[:])

	require.Equal(t, res, true)

}

func TestBTCLegacySignature(t *testing.T) {

	//m/44'/60'/0'/0/0 yellow ->
	privKey := base58.Decode("KxukKhTPU11xH2Wfk2366e375166QE4r7y8FWojU9XPbzLYYSM3j")

	message := "ACME will rule DEFI"
	hash := sha256.Sum256([]byte(message))
	secp := new(BTCLegacySignature)

	privkey, pbkey := btc.PrivKeyFromBytes(btc.S256(), privKey)

	secp.PublicKey = pbkey.SerializeUncompressed()

	require.NoError(t, SignBTCLegacy(secp, privkey.Serialize(), nil, hash[:]))
	res := secp.Verify(nil, hash[:])

	require.Equal(t, res, true)

}

func TestETHSignature(t *testing.T) {

	privKeyHex := "1b48e04041e23c72cacdaa9b0775d31515fc74d6a6d3c8804172f7e7d1248529"

	message := "ACME will rule DEFI"
	hash := sha256.Sum256([]byte(message))
	secp := new(ETHSignature)

	privKey, err := eth.HexToECDSA(privKeyHex)
	require.NoError(t, err)
	secp.PublicKey = eth.FromECDSAPub(&privKey.PublicKey)
	require.NoError(t, SignEthAsDer(secp, eth.FromECDSA(privKey), nil, hash[:]))

	t.Logf("Eth as Der public key  %x", secp.PublicKey)
	t.Logf("Eth as Der signature   %x", secp.Signature)
	t.Logf("Eth ad Der Hash        %x", hash[:])

	//should fail
	require.Equal(t, VerifyUserSignature(secp, hash[:]), false)
	//should pass
	require.Equal(t, VerifyUserSignatureV1(secp, hash[:]), true)

	//public key should still match
	keyComp, err := eth.UnmarshalPubkey(secp.PublicKey)

	require.NoError(t, err)
	require.True(t, keyComp.Equal(privKey.Public()), "public keys don't match")

	//version 2 signature test
	secp = new(ETHSignature)
	secp.PublicKey = eth.FromECDSAPub(&privKey.PublicKey)
	require.NoError(t, SignETH(secp, eth.FromECDSA(privKey), nil, hash[:]))

	t.Logf("Eth as VRS public key %x", secp.PublicKey)
	t.Logf("Eth as VRS signature  %x", secp.Signature)
	t.Logf("Eth ad VRS Hash       %x", hash[:])
	//should fail
	require.Equal(t, VerifyUserSignatureV1(secp, hash[:]), false)
	//should pass
	require.Equal(t, VerifyUserSignature(secp, hash[:]), true)

	t.Logf("Signature: %x", secp.Signature)
}

func TestBTCaddress(t *testing.T) {
	//m/44'/60'/0'/0/0 yellow ->
	//btc private address : "KxukKhTPU11xH2Wfk2366e375166QE4r7y8FWojU9XPbzLYYSM3j"
	pubKey, err := hex.DecodeString("02f7aa1eb14de438735c026c7cc719db11baf82e47f8fa2c86b55bff92b677eae2")
	require.NoError(t, err)
	address := "1Hdh7MEWekWD4qiHVRa2H8Ar3JR8sXunE"
	btcAddress := BTCaddress(pubKey)
	require.Equal(t, btcAddress, address)
}

func TestETHaddress(t *testing.T) {
	//m/44'/60'/0'/0/0 yellow ->
	// eth private address : "0x1b48e04041e23c72cacdaa9b0775d31515fc74d6a6d3c8804172f7e7d1248529"
	address := "0xa27df20e6579ac472481f0ea918165d24bfb713b"
	pubKey, err := hex.DecodeString("02c4755e0a7a0f7082749bf46cdae4fcddb784e11428446a01478d656f588f94c1")
	require.NoError(t, err)
	accEthAddress, err := ETHaddress(pubKey)
	require.NoError(t, err)
	require.Equal(t, address, accEthAddress)

	checkSum := sha256.Sum256([]byte(address[2:]))
	accEthLiteAccount, err := url.Parse(fmt.Sprintf("%s%x", address[2:], checkSum[28:]))
	require.NoError(t, err)
	lta, err := LiteTokenAddressFromHash(ETHhash(pubKey), ACME)
	require.NoError(t, err)
	require.Equal(t, accEthLiteAccount.JoinPath(ACME).String(), lta.String())
}

func mustDecodeHex(t testing.TB, s string) []byte {
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return b
}

func TestInitWithOtherKeys(t *testing.T) {
	ethPriv := mustDecodeHex(t, "1b48e04041e23c72cacdaa9b0775d31515fc74d6a6d3c8804172f7e7d1248529")
	_, ethPub := btc.PrivKeyFromBytes(btc.S256(), ethPriv)
	btcPriv := base58.Decode("KxukKhTPU11xH2Wfk2366e375166QE4r7y8FWojU9XPbzLYYSM3j")
	_, btcPub := btc.PrivKeyFromBytes(btc.S256(), btcPriv)
	btclPriv := base58.Decode("KxukKhTPU11xH2Wfk2366e375166QE4r7y8FWojU9XPbzLYYSM3j")
	_, btclPub := btc.PrivKeyFromBytes(btc.S256(), btclPriv)

	cases := map[string]struct {
		PrivKey []byte
		Signer  KeySignature
	}{
		"ETH":       {PrivKey: ethPriv, Signer: &ETHSignature{PublicKey: ethPub.SerializeUncompressed()}},
		"BTC":       {PrivKey: btcPriv, Signer: &BTCSignature{PublicKey: btcPub.SerializeCompressed()}},
		"BTCLegacy": {PrivKey: btclPriv, Signer: &BTCLegacySignature{PublicKey: btclPub.SerializeUncompressed()}},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			alice := url.MustParse("alice")

			// Initialize
			sim := NewSim(t,
				simulator.SimpleNetwork(t.Name(), 1, 1),
				simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2SignatureEthereum),
			)

			MakeIdentity(t, sim.DatabaseFor(alice), alice)
			UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
				p.CreditBalance = 1e9
				p.AddKeySpec(&KeySpec{PublicKeyHash: c.Signer.GetPublicKeyHash()})
			})

			env := MustBuild(t,
				build.Transaction().For(alice).
					CreateTokenAccount(alice, "tokens").ForToken(ACME).
					SignWith(alice, "book", "1").Version(1).Timestamp(1).Type(c.Signer.Type()).PrivateKey(c.PrivKey))

			// Confirm a simple hash was used and verify that it matches the
			// initiator
			require.Equal(t, env.Transaction[0].Header.Initiator[:], env.Signatures[0].Metadata().Hash())

			// Execute
			st := sim.SubmitTxnSuccessfully(env)
			sim.StepUntil(
				Txn(st.TxID).Succeeds())

			// Verify
			GetAccount[*TokenAccount](t, sim.DatabaseFor(alice), alice.JoinPath("tokens"))
		})
	}
}
