package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	btc "github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcutil/base58"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

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

	privKey, err := hex.DecodeString("1b48e04041e23c72cacdaa9b0775d31515fc74d6a6d3c8804172f7e7d1248529")
	require.NoError(t, err)
	message := "ACME will rule DEFI"
	hash := sha256.Sum256([]byte(message))
	secp := new(ETHSignature)

	privkey, pbkey := btc.PrivKeyFromBytes(btc.S256(), privKey)

	secp.PublicKey = pbkey.SerializeUncompressed()

	require.NoError(t, err)

	require.NoError(t, SignETH(secp, privkey.Serialize(), nil, hash[:]))
	res := secp.Verify(nil, hash[:])

	require.Equal(t, res, true)

}

func TestBTCaddress(t *testing.T) {
	//m/44'/60'/0'/0/0 yellow ->
	//privKey := "KxukKhTPU11xH2Wfk2366e375166QE4r7y8FWojU9XPbzLYYSM3j"
	pubKey, err := hex.DecodeString("02f7aa1eb14de438735c026c7cc719db11baf82e47f8fa2c86b55bff92b677eae2")
	require.NoError(t, err)
	address := "1Hdh7MEWekWD4qiHVRa2H8Ar3JR8sXunE"
	btcAddress := BTCaddress(pubKey)
	require.Equal(t, btcAddress, address)
}

func TestETHaddress(t *testing.T) {
	//m/44'/60'/0'/0/0 yellow ->
	//privKey := "0x1b48e04041e23c72cacdaa9b0775d31515fc74d6a6d3c8804172f7e7d1248529"
	address := "0xa27df20e6579ac472481f0ea918165d24bfb713b"
	pubKey, err := hex.DecodeString("02c4755e0a7a0f7082749bf46cdae4fcddb784e11428446a01478d656f588f94c1")
	require.NoError(t, err)
	accEthAddress, err := ETHaddress(pubKey)
	require.NoError(t, err)
	require.Equal(t, address, accEthAddress)

	checkSum := sha256.Sum256([]byte(address[2:]))
	accEthLiteAccount, err := url.Parse(fmt.Sprintf("%s%x", address[2:], checkSum[28:]))

	lta, err := LiteTokenAddressFromHash(ETHhash(pubKey), ACME)
	require.NoError(t, err)
	require.Equal(t, accEthLiteAccount.JoinPath(ACME).String(), lta.String())
}
