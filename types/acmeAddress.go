package types

import (
	"crypto/sha256"
	"strings"

	"github.com/martinlindhe/base36"
)

func GenerateAcmeAddress(pubKey []byte) string {
	keyHash := sha256.Sum256(pubKey[:])
	acme := []byte("acme")
	acmeKeyHash := append(acme, keyHash[:]...)
	checksum := sha256.Sum256(acmeKeyHash)
	rawAddress := append(keyHash[:], checksum[28:]...)
	encAddress := base36.EncodeBytes(rawAddress)
	address := string(acme) + strings.ToLower(encAddress)
	return address
}

func IsAcmeAddress(address *string) error {
	if !strings.HasPrefix(*address, "acme") {

	}

	//checksum...
	return nil
}
