// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package address

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"testing"

	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestGenerateKey(t *testing.T) {
	t.Skip("Manual")
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	addr := &PrivateKey{
		Key: sk,
		PublicKey: PublicKey{
			Type: protocol.SignatureTypeED25519,
			Key:  pk,
		},
	}
	fmt.Println(addr)

	// This is a bogus key that can be added to a key page
	fmt.Println()
	fmt.Println(&UnknownHash{
		Hash: []byte("The quick brown fox jumps over the lazy dog"),
	})

	a, err := Parse("MHz125hWDmTmFN25xqjfFTMdUyBRUDVyHcCn6Jp7NCbdjt45x4UQ9hg")
	require.NoError(t, err)
	fmt.Printf("%T\n", a)
	fmt.Println(string(a.(*UnknownMultihash).Digest))
}

func TestPublicKeyHash(t *testing.T) {
	cases := map[string]struct {
		String string
		Bytes  string
		Type   protocol.SignatureType
		Format func([]byte) string
	}{
		"AC1": {
			String: "AC12jWw3iqvdCqg11QTLFK9qdgABjknGUnLuX2sLiJMokWcAnqXFk",
			Bytes:  "e43be90e349210456662d8b8bdc9cc9e5e46ccb07f2129e7b57a8195e5e916d5",
			Type:   protocol.SignatureTypeED25519,
			Format: FormatAC1,
		},
		"AC2": {
			String: "AC2uQ9mAkcToGErGPunmXShG65PWCM9GdwD23LtVAx42WVjtHNsp",
			Bytes:  "76fa8bb661e3bd0f891d551bc764d35e15286b75903f60ae9fecb6dd75c79f73",
			Type:   protocol.SignatureTypeEcdsaSha256,
			Format: FormatAC2,
		},
		"AC3": {
			String: "AC36QP3SoTKgwfjgfMsqzJk9tGoDFeqMSc67jz1EpH61rGzEUJ7J",
			Bytes:  "0c44b67095f8fc86576269a542ce9f4fbeac4565d13e8fcb628faaec1368b779",
			Type:   protocol.SignatureTypeRsaSha256,
			Format: FormatAC3,
		},
		"FA": {
			String: "FA3hbVKd8MxyVhTqn7RwoWb2PXTiAbfhEbDSFCmTcmPYuQ8PdRGe",
			Bytes:  "e43be90e349210456662d8b8bdc9cc9e5e46ccb07f2129e7b57a8195e5e916d5",
			Type:   protocol.SignatureTypeRCD1,
			Format: FormatFA,
		},
		"MH": {
			String: "MHz126wLaU5sCEr5zFbyTt7Hg6pTTqSjaoSoXMnUg8izAmxvQ2xjXWL",
			Bytes:  "e43be90e349210456662d8b8bdc9cc9e5e46ccb07f2129e7b57a8195e5e916d5",
			Type:   protocol.SignatureTypeUnknown,
			Format: func(b []byte) string { return FormatMH(b, multihash.IDENTITY) },
		},
		"BTC": {
			String: "BT12jWw3iqvdCqg11QTLFK9qdgABjknGUnLuX2sLiJMokWcBt5rhQ",
			Bytes:  "e43be90e349210456662d8b8bdc9cc9e5e46ccb07f2129e7b57a8195e5e916d5",
			Type:   protocol.SignatureTypeBTC,
			Format: FormatBTC,
		},
		"ETH": {
			String: "0xe43be90e349210456662d8b8bdc9cc9e5e46ccb0",
			Bytes:  "e43be90e349210456662d8b8bdc9cc9e5e46ccb0",
			Type:   protocol.SignatureTypeETH,
			Format: FormatETH,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			bytes, err := hex.DecodeString(c.Bytes)
			require.NoError(t, err, "Invalid test case")

			t.Run("Format", func(t *testing.T) {
				s := c.Format(bytes)
				require.Equal(t, c.String, s, "Address must format correctly")
			})

			t.Run("Parse", func(t *testing.T) {
				addr, err := Parse(c.String)
				require.NoError(t, err, "Address must parse")
				require.Equal(t, c.Type, addr.GetType(), "Address must have the correct type")
				hash, ok := addr.GetPublicKeyHash()
				require.True(t, ok, "Address must provide a public key hash")
				require.Equal(t, bytes, hash, "Address must parse correctly")
			})

			t.Run("String", func(t *testing.T) {
				addr := &PublicKeyHash{Type: c.Type, Hash: bytes}
				require.Equal(t, c.String, addr.String(), "Address must format correctly")
			})
		})
	}
}

func TestPrivateKey(t *testing.T) {
	cases := map[string]struct {
		String string
		Seed   string
		Public string
		Type   protocol.SignatureType
		Format func([]byte) string
	}{
		"AS1": {
			String: "AS12jWw3iqvdCqg11QTLFK9qdgABjknGUnLuX2sLiJMokWcBq53Ux",
			Seed:   "e43be90e349210456662d8b8bdc9cc9e5e46ccb07f2129e7b57a8195e5e916d5",
			Public: "52813dbcd9e54c8bf504e83b3461355df1f8ed2f2e466c7f30dfd32ce7dbe79e",
			Type:   protocol.SignatureTypeED25519,
			Format: FormatAS1,
		},
		"AS2": {
			String: "AS24M2JMxB4uWnD2hsr16JbMrfJM1MbZsU2AumL9i3Yho9M26wKQpK3gyVATBH59rpK8AjkxMR3hRTmR6VPYWV9ZZDpj7YYs1wDg3QH56wS979TJYoRCbNTSVpwejVttDNQUd9qgMc5T8u5y1fvyXGpEBDPw1BBq6CscnYWwuxDSM4",
			Seed:   "307702010104205f6190b80e0fdcb89067f827d824cf004540d5dcb6a406efd67a41e97c7c401fa00a06082a8648ce3d030107a14403420004ffcf79dfc28b5e3039657adba9c00219520d54177d9b8c6ec9e0176702305070e2c41b674fc4e5e8951a188b8d6904bbee09d8b4cdd56e5259125b38f39d820f",
			Public: "3059301306072a8648ce3d020106082a8648ce3d03010703420004ffcf79dfc28b5e3039657adba9c00219520d54177d9b8c6ec9e0176702305070e2c41b674fc4e5e8951a188b8d6904bbee09d8b4cdd56e5259125b38f39d820f",
			Type:   protocol.SignatureTypeEcdsaSha256,
			Format: FormatAS2,
		},
		"AS3": {
			String: "AS3M5vNQ37taKs7v8wo9e6vop9NGJNwWvQd11uHnSbaJMbdiHJLyCD1p9iHJhRqbg3qtLSvfb8GTWgmh9yCjA2mpe3vpkxpqLEjpNVAAv4e6sYWP8Bwrp4DaQUqyccFiy5oT5CHhXopNtL26QFcS38rUHHY3mzpyPm4jmG1BRwdJWUGr6Q24tMcStaD9W1W9vK7wWwN2TvkwH4Dzcu6xfJcjFnszNQzwvLuSmDRfwC4tWgbN1uL6ajPJCnx6U1k8uaCYkJUyqjv37Q1GydfQ6d9NDuBpZ8YuFNKvRGT8t81zV4B4zqLCK7dLwtmkVX2mVkxe2YKi2M97AJMXTYu7GrhG9hUXLXxKtweXUQbBhELQPqUvGinch4TWuNh7n8RiSrjxuq3eeMSdpAdPLKaseYNfJEwexrk3E57BwLxzWWTBDppYVsZ7e3K1j58nXVdEXc8EeTu5mB5eaCoKaqtGvnVvCLW7a3dYjuBXJ9jqo3Bidb57L2juda6ywrpHCKUbD6BXNSnGeaf3ni8FGVx7omb3q8EDtH68HHE6tfDEgsw3MUAHVj5A37qQ993Ues96WKZqnZDRdbDcLcZiawfpstRQpmNminCtc94PJY3iZxL2SZU8ssjYBkY2tosnwgvucNpen5uaXuFEZcgEtNjF4KoppjW6Ybyfj4GkKaQiKkwbY7CcrwD6bPG8zjPkp1tCAVJ994hceAKDejQB96Lx2apeZ4jpaNeq2zY8QYikGqajigu3Eom3XvgD1zkAUpCVX1pBKRfczwbBg1D6TfbFJ2P48EM8sTxquPJRKueDq1K7WfqrbLPtrUrysFGHTLJcyXBPKSd7",
			Seed:   "3082025d02010002818100a0037fa2435ff361170a0004f3ff2f8a6005b46e887d54610515a9a59e17b623616e12dcb86c51f1bbd9edcc84b9bb5abc3a5e3d9fda601c5f341b8b1c20a6d6a91d8cf26d1cf58b8d0de8e45db5bf22a7f6b59c248c0ae5305711c60bde3df8ea65752752aa6ed27f00d41350deea394b546c2bd1afc1b56f39c2eeb65ccb8302030100010281802ff01c62ce70868785d1772404bd64cebde9b79e8dc18e6fea880ce51331e35d422929f97a3ee965d4737bac93eed8d45c23d86bf9e2007d3f806ae8082b3810094dffec4202717d48cea3b381aee3132d4ecb5ab0ecb228dfe7961504ceba5b7132ebfdc26d8d77d99e26a31669b852bcfb9b37655b9ad42f201c67983f3cd1024100e366e5de47aaa5c714448c0c0a928bb8cd1b92bbde6108951ff56def6a5f08ec24e4bf293be718a57f00e7ceab9ab11a75e317400c7cbcd535bb5a9e37eee54f024100b42310d18516aecbdfb6de0795ae7179f733ca45bca50ff4ec5cde37a70dfda8e01f74162718a9a1725927e3fd5ef152eb43cc5e1c84d221e339fcabe97dd18d0241008b5bccf57ad00e86415a7cc50859a4bcdd1dcfbfc85988ebf2c8f2d56d47ce6d017319df883168be4b65dfa99d2ad0ec59609128143f016e5c5e73fb5065dc2b024014c2a9d08bd7166416af5b9d6229cd9831de3d24965e2b9d9d94026445937e3575470ab8f6de87c63c887be971acfc8102ee38626c44fe76bb5a567d7596738d024100aac157accbc999755d49820944b2bb65bcf70ad1afe455942a3c81a9b617d50f9eb8f3a1b0553c321fa4024c3fd9c4c7e270bcf0f83a535e245977d507ca8051",
			Public: "30818902818100a0037fa2435ff361170a0004f3ff2f8a6005b46e887d54610515a9a59e17b623616e12dcb86c51f1bbd9edcc84b9bb5abc3a5e3d9fda601c5f341b8b1c20a6d6a91d8cf26d1cf58b8d0de8e45db5bf22a7f6b59c248c0ae5305711c60bde3df8ea65752752aa6ed27f00d41350deea394b546c2bd1afc1b56f39c2eeb65ccb830203010001",
			Type:   protocol.SignatureTypeRsaSha256,
			Format: FormatAS3,
		},
		"Fs": {
			String: "Fs342EuYBZJ7TbmZLpN2q1nHVVxsQdtJ8wu3uZxiyVojCvfUGwUg",
			Seed:   "e43be90e349210456662d8b8bdc9cc9e5e46ccb07f2129e7b57a8195e5e916d5",
			Public: "52813dbcd9e54c8bf504e83b3461355df1f8ed2f2e466c7f30dfd32ce7dbe79e",
			Type:   protocol.SignatureTypeRCD1,
			Format: FormatFs,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			seed, err := hex.DecodeString(c.Seed)
			require.NoError(t, err, "Invalid test case")
			public, err := hex.DecodeString(c.Public)
			require.NoError(t, err, "Invalid test case")

			t.Run("Format", func(t *testing.T) {
				s := c.Format(seed)
				require.Equal(t, c.String, s, "Address must format correctly")
			})

			t.Run("Parse", func(t *testing.T) {
				addr, err := Parse(c.String)
				require.NoError(t, err, "Address must parse")
				require.Equal(t, c.Type, addr.GetType(), "Address must have the correct type")
				priv, ok := addr.GetPrivateKey()
				require.True(t, ok, "Address must provide a private key")
				if c.Type != protocol.SignatureTypeRsaSha256 && c.Type != protocol.SignatureTypeEcdsaSha256 {
					require.Equal(t, append(seed, public...), priv, "Address must parse correctly")
				} else {
					require.Equal(t, seed, priv, "Address must parse correctly")
				}
			})

			t.Run("String", func(t *testing.T) {
				if c.Type != protocol.SignatureTypeRsaSha256 && c.Type != protocol.SignatureTypeEcdsaSha256 {
					addr := &PrivateKey{PublicKey: PublicKey{Type: c.Type, Key: public}, Key: append(seed, public...)}
					require.Equal(t, c.String, addr.String(), "Address must format correctly")
				} else {
					addr := &PrivateKey{PublicKey: PublicKey{Type: c.Type, Key: public}, Key: seed}
					require.Equal(t, c.String, addr.String(), "Address must format correctly")
				}
			})
		})
	}
}

func TestFormatWif(t *testing.T) {
	//BITCOIN Wallet Import Format (WIF), all keys start with 5, K, or L
	wif := "L3qjEroY5veLKTaeRdhuXScuYcXYy3CaSqeQVfvDmZKV5kRDuLWv"
	raw := "c58db2514ae2079d456b586223242c5f61c73a5a2406dd2946ed76e06422292e"

	privateKey, _, err := parseWIF(wif)

	require.NoError(t, err)

	a, err := Parse(wif)
	require.NoError(t, err)

	rawa, _ := a.GetPrivateKey()
	require.Equal(t, hex.EncodeToString(rawa), raw)
	pk, err := FromPrivateKeyBytes(privateKey, protocol.SignatureTypeBTC)
	require.NoError(t, err)

	require.Equal(t, a.String(), pk.String())

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

func TestFormatAC3(t *testing.T) {
	t.Skip("Manual")
	block, _ := pem.Decode([]byte(rsaPrivateKey1024))

	pk, err := FromPrivateKeyBytes(block.Bytes, protocol.SignatureTypeRsaSha256)
	require.NoError(t, err)
	//AC36QP3SoTKgwfjgfMsqzJk9tGoDFeqMSc67jz1EpH61rGzEUJ7J
	t.Logf("seed: %x", block.Bytes)
	t.Logf(pk.String())
	t.Logf(pk.PublicKey.String())
	kh, _ := pk.PublicKey.GetPublicKeyHash()
	t.Logf("key hash: %x", kh)
	t.Logf("%x", pk.PublicKey.Key)
}

func TestFormatAC2(t *testing.T) {
	t.Skip("Manual")
	// Generate ECDSA private key
	ecdsaPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	pk := FromEcdsaPrivateKey(ecdsaPrivKey)
	t.Logf("seed: %x", pk.Key)
	t.Logf(pk.String())
	t.Logf(pk.PublicKey.String())
	kh, _ := pk.PublicKey.GetPublicKeyHash()
	t.Logf("%x", pk.PublicKey.Key)
	t.Logf("key hash: %x", kh)
}
