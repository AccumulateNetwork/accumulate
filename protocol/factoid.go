package protocol

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"github.com/btcsuite/btcutil/base58"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"strings"
)

func GetRCDFromFactoidAddress(fa string) ([]byte, error) {

	if len(fa) != 52 {
		return nil, fmt.Errorf("invalid factoid address length")
	}

	if !strings.HasPrefix(fa, "FA") {
		return nil, fmt.Errorf("invalid factoid address prefix, expecting FA but received %s", fa[:2])
	}

	faPrefix := []byte{0x5f, 0xb1}
	decodedFA := base58.Decode(fa)

	if !bytes.HasPrefix(decodedFA[:2], faPrefix) {
		return nil, fmt.Errorf("invalid factoid base58 encoding prefix")
	}

	checksum := sha256.Sum256(decodedFA[:34])
	checksum = sha256.Sum256(checksum[:])
	if !bytes.HasSuffix(decodedFA[34:], checksum[:4]) {
		return nil, fmt.Errorf("invalid checksum on factoid address")
	}

	rcdHash := decodedFA[2:34]
	return rcdHash, nil
}

func GetLiteAccountFromFactoidAddress(fa string) (*url.URL, error) {
	rcdHash, err := GetRCDFromFactoidAddress(fa)
	if err != nil {
		return nil, err
	}
	return LiteTokenAddress(rcdHash, ACME)
}
