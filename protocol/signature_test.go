// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol_test

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
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
				simulator.Genesis(GenesisTime),
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

const rsaPrivateKey1024 = "-----BEGIN RSA PRIVATE KEY-----\nMIICXQIBAAKBgQCgA3+iQ1/zYRcKAATz/y+KYAW0boh9VGEFFamlnhe2I2FuEty4\nbFHxu9ntzIS5u1q8Ol49n9pgHF80G4scIKbWqR2M8m0c9YuNDejkXbW/Iqf2tZwk\njArlMFcRxgvePfjqZXUnUqpu0n8A1BNQ3uo5S1RsK9GvwbVvOcLutlzLgwIDAQAB\nAoGAL/AcYs5whoeF0XckBL1kzr3pt56NwY5v6ogM5RMx411CKSn5ej7pZdRze6yT\n7tjUXCPYa/niAH0/gGroCCs4EAlN/+xCAnF9SM6js4Gu4xMtTstasOyyKN/nlhUE\nzrpbcTLr/cJtjXfZniajFmm4Urz7mzdlW5rULyAcZ5g/PNECQQDjZuXeR6qlxxRE\njAwKkou4zRuSu95hCJUf9W3val8I7CTkvyk75xilfwDnzquasRp14xdADHy81TW7\nWp437uVPAkEAtCMQ0YUWrsvftt4Hla5xefczykW8pQ/07FzeN6cN/ajgH3QWJxip\noXJZJ+P9XvFS60PMXhyE0iHjOfyr6X3RjQJBAItbzPV60A6GQVp8xQhZpLzdHc+/\nyFmI6/LI8tVtR85tAXMZ34gxaL5LZd+pnSrQ7FlgkSgUPwFuXF5z+1Bl3CsCQBTC\nqdCL1xZkFq9bnWIpzZgx3j0kll4rnZ2UAmRFk341dUcKuPbeh8Y8iHvpcaz8gQLu\nOGJsRP52u1pWfXWWc40CQQCqwVesy8mZdV1JgglEsrtlvPcK0a/kVZQqPIGpthfV\nD56486GwVTwyH6QCTD/ZxMficLzw+DpTXiRZd9UHyoBR\n-----END RSA PRIVATE KEY-----"
const rsaPrivateKey2048 = "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEAn0hLBj7BdMbm5w0iIK3dGSGoHNR9R8H9mYaRkkomKmQAcz7E\nuuBVHco2I1966Vx7P04L/Rx3KNo//6QhOoFLeYSi0Q7+xit/NVGQejz/J8jKXKs6\nlanUUxefrGnmsAqj+folOnjrHlBHsjzJcmKLvpou8Sf2bSdeLZs15FufQWoDeVv+\nxUPllQZghcg8Bbu814Wl+wFlLSN1eaizUbhxzhCmMVfhsnuFYGBqm6sjUVv/R/Ty\nxMdnfTLWgPDbWSWg0JQiW4vD5b1XEHPFz209+uCVHtXKlo9d7mf20X4bwuvV3nM6\nCDDFi0CIY5ZNbq5cYnD8Ftuz2qCaplRKtiZofwIDAQABAoIBAD2GrEw+Q3X7Osf3\nH76lyijiAlEYl0f3nCEIhQSQFcv8EtxxW4agDuDR8jWZtR2dNpJOcH0V2MV0AJKb\n8KXruZ636Dh+5VThCmMrHXbKRvk0K06+aYPUNQrfrjLoOU643Xw67tR2TsPH2Nn1\ndw7zF+3JGubWO+8P7OYK9Tc/WPXoA2OPtGvQdEQlHAGkJS4u6Gj7u1Wq1BKOwjJJ\nQ04TsGhv5U4epVaKMdgCGEY8DZNwf/xkLz6gSLL3gJivlEkEz3zsMLy/CgssnZQd\ndzzsF+o5UC+TCZNpbOijweL5rtQh/7gdn9qNNEfdRmIfSdTRltXSYkjRWZWVR1Cd\nvUkUs5kCgYEA4pY9PVJwxyrXy3DMyksx6vsHlidLBOxy7RgbE/hwkM72gIG8Fso5\nHXMsDTnzAidOZYX6paCGbwf2pMxUpAIhJiJuG6so2KYGORgN0kwceUoya8pOvuGZ\nKMllT9cr25VPsfsGP4Sj9DazTrUjXgcbd1ysEdUFBaXurmwRqH0WgNMCgYEAs/Vx\ndN7t78Jme7EZRJ+MB0btCLlTuGPaTNkMh4UjnDrLK6mPzfgxAilgAgBH/jhm8OOK\nKA+JsmppomiID+URZeGii7h9JJr0V1TjMSUsk7jJzhsQyaWe9o6hKWHbtocOtQXi\nylshJpa5B8/x9bQhxMeLVTxzyRX0QygJcBtUziUCgYEAiYx6kIdDPySa6z0GlKch\nHmxVJqmjuNFw0s0XYwAmFUIOEeSvsYYBNgd8bmsHQf9qb+btSS4xbaV/7Hq9xvIj\n/WpZPSKiISJoFLCtc0QQ5PBNu3GMbAO3XjMj9VvBnAL/5iNkn5p9jPrHzrfXSHU4\nDzWKnyiZa9xXEDs6XPXSe1ECgYBkd6a7xKm5rSJh8+FTem9GsMYslKq0yqpZNOPV\n1PKoifpbifKK3wEdX9QFyfpnZz2xRpce/m21ecs3rHwpw40PAAUrU/gps4iuKOod\nyc81OXkQ4/NfYGN66u32mHd9U7FWRs7ygiXj0UnDnshKkCI6Jd0X3QQXQ3Z296ct\nO1UBMQKBgBo0uw7h63SrJo9QBsDHzgaLW6MHBu7YodeE3844UoMtiKZ2jfN9AOOp\nWATvaUxZ6qjUjBMw2dM7j2quqrFS/Orgn/3cxIoA3Lllwx15W21vBHha8iMdGRa/\nuThEun/KbtsVzrNADmJZawAgE7dIFVNsetkvPbzR0LIiJSYHvPj+\n-----END RSA PRIVATE KEY-----\n"
const rsaPrivateKey4096 = "-----BEGIN RSA PRIVATE KEY-----\n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\n-----END RSA PRIVATE KEY-----"

func TestRsaSha256Signature(t *testing.T) {
	message := "ACME will rule DEFI"
	hash := sha256.Sum256([]byte(message))
	block, _ := pem.Decode([]byte(rsaPrivateKey1024))
	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)

	require.NoError(t, err)

	rsaSha256 := new(RsaSha256Signature)
	rsaSha256.PublicKey, err = x509.MarshalPKIXPublicKey(privKey.Public())
	require.NoError(t, err)

	require.NoError(t, SignRsaSha256(rsaSha256, x509.MarshalPKCS1PrivateKey(privKey), nil, hash[:]))

	//should fail
	require.Equal(t, VerifyUserSignature(rsaSha256, hash[:]), true)
	//public key should still match
	keyComp, err := x509.ParsePKIXPublicKey(rsaSha256.PublicKey)
	require.NoError(t, err)

	require.True(t, keyComp.(*rsa.PublicKey).Equal(privKey.Public()), "public keys don't match")
}
