package abci_test

import (
	"crypto/ed25519"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	mock_api "github.com/AccumulateNetwork/accumulate/internal/mock/api"
	"github.com/golang/mock/gomock"
	"io"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
)

func generateKey() tmed25519.PrivKey {
	_, key, _ := ed25519.GenerateKey(rand)
	return tmed25519.PrivKey(key)
}

func edSigner(key tmed25519.PrivKey, nonce uint64) func(hash []byte) (*transactions.ED25519Sig, error) {
	return func(hash []byte) (*transactions.ED25519Sig, error) {
		sig := new(transactions.ED25519Sig)
		return sig, sig.Sign(nonce, key, hash)
	}
}
