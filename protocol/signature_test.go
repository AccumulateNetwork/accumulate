// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol_test

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"testing"
	"time"

	btc "github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcutil/base58"
	eth "github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
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
	addr := "1Hdh7MEWekWD4qiHVRa2H8Ar3JR8sXunE"
	btcAddress := BTCaddress(pubKey)
	require.Equal(t, btcAddress, addr)
}

func TestETHaddress(t *testing.T) {
	//m/44'/60'/0'/0/0 yellow ->
	// eth private address : "0x1b48e04041e23c72cacdaa9b0775d31515fc74d6a6d3c8804172f7e7d1248529"
	addr := "0xa27df20e6579ac472481f0ea918165d24bfb713b"
	pubKey, err := hex.DecodeString("02c4755e0a7a0f7082749bf46cdae4fcddb784e11428446a01478d656f588f94c1")
	require.NoError(t, err)
	accEthAddress, err := ETHaddress(pubKey)
	require.NoError(t, err)
	require.Equal(t, addr, accEthAddress)

	checkSum := sha256.Sum256([]byte(addr[2:]))
	accEthLiteAccount, err := url.Parse(fmt.Sprintf("%s%x", addr[2:], checkSum[28:]))
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

const rsaPrivateKey1024 = `-----BEGIN RSA PRIVATE KEY-----
MIICXQIBAAKBgQCgA3+iQ1/zYRcKAATz/y+KYAW0boh9VGEFFamlnhe2I2FuEty4
bFHxu9ntzIS5u1q8Ol49n9pgHF80G4scIKbWqR2M8m0c9YuNDejkXbW/Iqf2tZwk
jArlMFcRxgvePfjqZXUnUqpu0n8A1BNQ3uo5S1RsK9GvwbVvOcLutlzLgwIDAQAB
AoGAL/AcYs5whoeF0XckBL1kzr3pt56NwY5v6ogM5RMx411CKSn5ej7pZdRze6yT
7tjUXCPYa/niAH0/gGroCCs4EAlN/+xCAnF9SM6js4Gu4xMtTstasOyyKN/nlhUE
zrpbcTLr/cJtjXfZniajFmm4Urz7mzdlW5rULyAcZ5g/PNECQQDjZuXeR6qlxxRE
jAwKkou4zRuSu95hCJUf9W3val8I7CTkvyk75xilfwDnzquasRp14xdADHy81TW7
Wp437uVPAkEAtCMQ0YUWrsvftt4Hla5xefczykW8pQ/07FzeN6cN/ajgH3QWJxip
oXJZJ+P9XvFS60PMXhyE0iHjOfyr6X3RjQJBAItbzPV60A6GQVp8xQhZpLzdHc+/
yFmI6/LI8tVtR85tAXMZ34gxaL5LZd+pnSrQ7FlgkSgUPwFuXF5z+1Bl3CsCQBTC
qdCL1xZkFq9bnWIpzZgx3j0kll4rnZ2UAmRFk341dUcKuPbeh8Y8iHvpcaz8gQLu
OGJsRP52u1pWfXWWc40CQQCqwVesy8mZdV1JgglEsrtlvPcK0a/kVZQqPIGpthfV
D56486GwVTwyH6QCTD/ZxMficLzw+DpTXiRZd9UHyoBR
-----END RSA PRIVATE KEY-----`

const rsaPrivateKey2048 = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAn0hLBj7BdMbm5w0iIK3dGSGoHNR9R8H9mYaRkkomKmQAcz7E
uuBVHco2I1966Vx7P04L/Rx3KNo//6QhOoFLeYSi0Q7+xit/NVGQejz/J8jKXKs6
lanUUxefrGnmsAqj+folOnjrHlBHsjzJcmKLvpou8Sf2bSdeLZs15FufQWoDeVv+
xUPllQZghcg8Bbu814Wl+wFlLSN1eaizUbhxzhCmMVfhsnuFYGBqm6sjUVv/R/Ty
xMdnfTLWgPDbWSWg0JQiW4vD5b1XEHPFz209+uCVHtXKlo9d7mf20X4bwuvV3nM6
CDDFi0CIY5ZNbq5cYnD8Ftuz2qCaplRKtiZofwIDAQABAoIBAD2GrEw+Q3X7Osf3
H76lyijiAlEYl0f3nCEIhQSQFcv8EtxxW4agDuDR8jWZtR2dNpJOcH0V2MV0AJKb
8KXruZ636Dh+5VThCmMrHXbKRvk0K06+aYPUNQrfrjLoOU643Xw67tR2TsPH2Nn1
dw7zF+3JGubWO+8P7OYK9Tc/WPXoA2OPtGvQdEQlHAGkJS4u6Gj7u1Wq1BKOwjJJ
Q04TsGhv5U4epVaKMdgCGEY8DZNwf/xkLz6gSLL3gJivlEkEz3zsMLy/CgssnZQd
dzzsF+o5UC+TCZNpbOijweL5rtQh/7gdn9qNNEfdRmIfSdTRltXSYkjRWZWVR1Cd
vUkUs5kCgYEA4pY9PVJwxyrXy3DMyksx6vsHlidLBOxy7RgbE/hwkM72gIG8Fso5
HXMsDTnzAidOZYX6paCGbwf2pMxUpAIhJiJuG6so2KYGORgN0kwceUoya8pOvuGZ
KMllT9cr25VPsfsGP4Sj9DazTrUjXgcbd1ysEdUFBaXurmwRqH0WgNMCgYEAs/Vx
dN7t78Jme7EZRJ+MB0btCLlTuGPaTNkMh4UjnDrLK6mPzfgxAilgAgBH/jhm8OOK
KA+JsmppomiID+URZeGii7h9JJr0V1TjMSUsk7jJzhsQyaWe9o6hKWHbtocOtQXi
ylshJpa5B8/x9bQhxMeLVTxzyRX0QygJcBtUziUCgYEAiYx6kIdDPySa6z0GlKch
HmxVJqmjuNFw0s0XYwAmFUIOEeSvsYYBNgd8bmsHQf9qb+btSS4xbaV/7Hq9xvIj
/WpZPSKiISJoFLCtc0QQ5PBNu3GMbAO3XjMj9VvBnAL/5iNkn5p9jPrHzrfXSHU4
DzWKnyiZa9xXEDs6XPXSe1ECgYBkd6a7xKm5rSJh8+FTem9GsMYslKq0yqpZNOPV
1PKoifpbifKK3wEdX9QFyfpnZz2xRpce/m21ecs3rHwpw40PAAUrU/gps4iuKOod
yc81OXkQ4/NfYGN66u32mHd9U7FWRs7ygiXj0UnDnshKkCI6Jd0X3QQXQ3Z296ct
O1UBMQKBgBo0uw7h63SrJo9QBsDHzgaLW6MHBu7YodeE3844UoMtiKZ2jfN9AOOp
WATvaUxZ6qjUjBMw2dM7j2quqrFS/Orgn/3cxIoA3Lllwx15W21vBHha8iMdGRa/
uThEun/KbtsVzrNADmJZawAgE7dIFVNsetkvPbzR0LIiJSYHvPj+
-----END RSA PRIVATE KEY-----`
const rsaPrivateKey4096 = `-----BEGIN RSA PRIVATE KEY-----
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
-----END RSA PRIVATE KEY-----`

func TestRsaSha256Signature(t *testing.T) {
	message := "ACME will rule DEFI"
	hash := sha256.Sum256([]byte(message))
	block, _ := pem.Decode([]byte(rsaPrivateKey1024))
	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)

	require.NoError(t, err)

	rsaSha256 := new(RsaSha256Signature)
	rsaSha256.PublicKey = x509.MarshalPKCS1PublicKey(&privKey.PublicKey)

	require.NoError(t, SignRsaSha256(rsaSha256, x509.MarshalPKCS1PrivateKey(privKey), nil, hash[:]))

	//should fail
	require.Equal(t, VerifyUserSignature(rsaSha256, hash[:]), true)
	//public key should still match
	keyComp, err := x509.ParsePKCS1PublicKey(rsaSha256.PublicKey)
	require.NoError(t, err)

	require.True(t, keyComp.Equal(privKey.Public()), "public keys don't match")

	//now try 2048 key

	block, _ = pem.Decode([]byte(rsaPrivateKey2048))
	privKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	require.NoError(t, err)

	rsaSha256 = new(RsaSha256Signature)
	rsaSha256.PublicKey = x509.MarshalPKCS1PublicKey(&privKey.PublicKey)

	require.NoError(t, SignRsaSha256(rsaSha256, x509.MarshalPKCS1PrivateKey(privKey), nil, hash[:]))

	//should fail
	require.Equal(t, VerifyUserSignature(rsaSha256, hash[:]), true)
	//public key should still match
	keyComp, err = x509.ParsePKCS1PublicKey(rsaSha256.PublicKey)
	require.NoError(t, err)

	require.True(t, keyComp.Equal(privKey.Public()), "public keys don't match")

	//now try 4096 key

	block, _ = pem.Decode([]byte(rsaPrivateKey4096))
	privKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	require.NoError(t, err)

	rsaSha256 = new(RsaSha256Signature)
	rsaSha256.PublicKey = x509.MarshalPKCS1PublicKey(&privKey.PublicKey)

	require.NoError(t, SignRsaSha256(rsaSha256, x509.MarshalPKCS1PrivateKey(privKey), nil, hash[:]))

	//should fail
	require.Equal(t, VerifyUserSignature(rsaSha256, hash[:]), true)
	//public key should still match
	keyComp, err = x509.ParsePKCS1PublicKey(rsaSha256.PublicKey)
	require.NoError(t, err)

	require.True(t, keyComp.Equal(privKey.Public()), "public keys don't match")

}

func generateTestPkiCertificates() (string, string, string, error) {
	certTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "example.com",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Generate RSA private key
	rsaPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate RSA key: %v", err)
	}
	rsaCertBytes, err := x509.CreateCertificate(rand.Reader, &certTemplate, &certTemplate, &rsaPrivKey.PublicKey, rsaPrivKey)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create RSA certificate: %v", err)
	}
	rsaCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rsaCertBytes})
	rsaPrivKeyPEM, err := x509.MarshalPKCS8PrivateKey(rsaPrivKey)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to marshal RSA private key: %v", err)
	}
	rsaPrivKeyPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: rsaPrivKeyPEM})

	// Generate ECDSA private key
	ecdsaPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate ECDSA key: %v", err)
	}
	ecdsaCertBytes, err := x509.CreateCertificate(rand.Reader, &certTemplate, &certTemplate, &ecdsaPrivKey.PublicKey, ecdsaPrivKey)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create ECDSA certificate: %v", err)
	}
	ecdsaCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ecdsaCertBytes})
	ecdsaPrivKeyPEM, err := x509.MarshalPKCS8PrivateKey(ecdsaPrivKey)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to marshal ECDSA private key: %v", err)
	}
	ecdsaPrivKeyPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecdsaPrivKeyPEM})

	// Generate Ed25519 private key
	_, ed25519PrivKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate Ed25519 key: %v", err)
	}
	ed25519CertBytes, err := x509.CreateCertificate(rand.Reader, &certTemplate, &certTemplate, ed25519PrivKey.Public().(ed25519.PublicKey), ed25519PrivKey)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create Ed25519 certificate: %v", err)
	}
	ed25519CertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ed25519CertBytes})
	ed25519PrivKeyPEM, err := x509.MarshalPKCS8PrivateKey(ed25519PrivKey)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to marshal Ed25519 private key: %v", err)
	}
	ed25519PrivKeyPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ed25519PrivKeyPEM})

	return string(rsaCertPEM) + string(rsaPrivKeyPEMBytes), string(ecdsaCertPEM) + string(ecdsaPrivKeyPEMBytes), string(ed25519CertPEM) + string(ed25519PrivKeyPEMBytes), nil
}

func TestTypesFromCerts(t *testing.T) {
	rsaCert, ecdsaCert, ed25519Cert, err := generateTestPkiCertificates()
	if err != nil {
		t.Fatalf("Failed to generate certificates: %v\n", err)
	}

	message := "ACME will rule DEFI"
	hash := sha256.Sum256([]byte(message))

	pub := &address.PublicKey{}

	var sig UserSignature
	for _, c := range []string{rsaCert, ecdsaCert, ed25519Cert} {
		block, rest := pem.Decode([]byte(c))
		if block.Type == "CERTIFICATE" {
			block, _ = pem.Decode(rest)
		}
		require.Equal(t, block.Type, "PRIVATE KEY")
		privKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		require.NoError(t, err)
		switch k := privKey.(type) {
		case *rsa.PrivateKey:
			pk := address.FromPrivateKeyBytes(block.Bytes, SignatureTypeRsaSha256)
			pub = address.FromRSAPublicKey(&k.PublicKey)
			require.Equal(t, pk.PublicKey.Key, pub.Key)
			priv := address.FromRSAPrivateKey(k)
			s := new(RsaSha256Signature)
			s.PublicKey = pub.Key
			require.NoError(t, SignRsaSha256(s, priv.Key, nil, hash[:]))
			sig = s
		case *ecdsa.PrivateKey:
			pk := address.FromPrivateKeyBytes(block.Bytes, SignatureTypeEcdsaSha256)
			pub = address.FromEcdsaPublicKeyAsPKIX(&k.PublicKey)
			require.Equal(t, pk.PublicKey.Key, pub.Key)
			priv := address.FromEcdsaPrivateKey(k)
			s := new(EcdsaSha256Signature)
			s.PublicKey = pub.Key
			require.NoError(t, SignEcdsaSha256(s, priv.Key, nil, hash[:]))
			sig = s
		case ed25519.PrivateKey:
			pk := address.FromPrivateKeyBytes(block.Bytes, SignatureTypeED25519)
			pub = address.FromED25519PublicKey(k.Public().(ed25519.PublicKey))
			require.Equal(t, pk.PublicKey.Key, pub.Key)
			priv := address.FromED25519PrivateKey(k)
			s := new(ED25519Signature)
			s.PublicKey = pub.Key
			SignED25519(s, priv.Key, nil, hash[:])
			sig = s
		default:
			continue
		}

		//should not fail
		require.Equal(t, VerifyUserSignature(sig, hash[:]), true)
	}
}
