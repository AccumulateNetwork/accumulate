package protocol

import (
	"crypto/sha256"
	"testing"

	btc "github.com/btcsuite/btcd/btcec"
	"github.com/stretchr/testify/require"
)

func TestBTCSignature(t *testing.T) {

	message := "ACME will rule DEFI"
	hash := sha256.Sum256([]byte(message))
	secp := new(BTCSignature)

	pk, err := btc.NewPrivateKey(btc.S256())
	pkBytes := pk.Serialize()
	privkey, pbkey := btc.PrivKeyFromBytes(btc.S256(), pkBytes)

	secp.PublicKey = pbkey.SerializeCompressed()

	require.NoError(t, err)

	require.NoError(t, SignBTC(secp, privkey.Serialize(), hash[:]))
	res := secp.Verify(nil, hash[:])

	require.Equal(t, res, true)

}

func TestBTCLegacySignature(t *testing.T) {

	message := "ACME will rule DEFI"
	hash := sha256.Sum256([]byte(message))
	secp := new(BTCLegacySignature)

	pk, err := btc.NewPrivateKey(btc.S256())
	pkBytes := pk.Serialize()
	privkey, pbkey := btc.PrivKeyFromBytes(btc.S256(), pkBytes)

	secp.PublicKey = pbkey.SerializeUncompressed()

	require.NoError(t, err)

	require.NoError(t, SignBTCLegacy(secp, privkey.Serialize(), hash[:]))
	res := secp.Verify(nil, hash[:])

	require.Equal(t, res, true)

}

func TestETHSignature(t *testing.T) {

	message := "ACME will rule DEFI"
	hash := sha256.Sum256([]byte(message))
	secp := new(ETHSignature)

	pk, err := btc.NewPrivateKey(btc.S256())
	pkBytes := pk.Serialize()
	privkey, pbkey := btc.PrivKeyFromBytes(btc.S256(), pkBytes)

	secp.PublicKey = pbkey.SerializeUncompressed()

	require.NoError(t, err)

	require.NoError(t, SignETH(secp, privkey.Serialize(), hash[:]))
	res := secp.Verify(nil, hash[:])

	require.Equal(t, res, true)

}
