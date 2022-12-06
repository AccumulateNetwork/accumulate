package address

import (
	"encoding/hex"
	"testing"

	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestPublicKeyHash(t *testing.T) {
	cases := map[string]struct {
		String string
		Bytes  string
		Type   protocol.SignatureType
		Format func([]byte) string
	}{
		"AC": {
			String: "AC12jWw3iqvdCqg11QTLFK9qdgABjknGUnLuX2sLiJMokWcAnqXFk",
			Bytes:  "e43be90e349210456662d8b8bdc9cc9e5e46ccb07f2129e7b57a8195e5e916d5",
			Type:   protocol.SignatureTypeED25519,
			Format: FormatAC1,
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
		"AS": {
			String: "AS12jWw3iqvdCqg11QTLFK9qdgABjknGUnLuX2sLiJMokWcBq53Ux",
			Seed:   "e43be90e349210456662d8b8bdc9cc9e5e46ccb07f2129e7b57a8195e5e916d5",
			Public: "52813dbcd9e54c8bf504e83b3461355df1f8ed2f2e466c7f30dfd32ce7dbe79e",
			Type:   protocol.SignatureTypeED25519,
			Format: FormatAS1,
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
				hash, ok := addr.GetPrivateKey()
				require.True(t, ok, "Address must provide a private key")
				require.Equal(t, append(seed, public...), hash, "Address must parse correctly")
			})

			t.Run("String", func(t *testing.T) {
				addr := &PrivateKey{PublicKey: PublicKey{Type: c.Type, Key: public}, Key: append(seed, public...)}
				require.Equal(t, c.String, addr.String(), "Address must format correctly")
			})
		})
	}
}
