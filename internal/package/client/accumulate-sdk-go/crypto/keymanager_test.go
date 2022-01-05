package crypto_test

import (
	"testing"

	"github.com/AccumulateNetwork/accumulate/internal/package/client/accumulate-sdk-go/crypto"
	"github.com/stretchr/testify/assert"
)


func TestNewMnemonicKeyManager(t *testing.T) {
	mnemonic := "today winning think single spice task car start piece full run hospital control inside cousin romance left choice poet wagon rude climb leisure spring"

	km, err := crypto.NewMnemonicManager(mnemonic)
	assert.NoError(t, err)
	t.Log(km)

}