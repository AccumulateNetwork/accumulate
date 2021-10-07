package api

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/stretchr/testify/require"
)

func TestTokenAccount(t *testing.T) {
	tokenUrl := types.String("roadrunner/BeepBeep")
	adiChainPath := types.String("WileECoyote/ACME")
	_, chain, err := types.ParseIdentityChainPath(adiChainPath.AsString())
	require.NoError(t, err)

	account := NewTokenAccount(types.String(chain), tokenUrl)
	require.Equal(t, tokenUrl, account.TokenURL)
	require.Equal(t, chain, *account.URL.AsString())

	data, err := json.Marshal(account)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("%s\n", string(data))

	t.Run("JSON", func(t *testing.T) {
		data, err := json.Marshal(account)
		require.NoError(t, err, "Marshalling")

		unmarshalled := new(TokenAccount)
		err = json.Unmarshal(data, &unmarshalled)
		require.NoError(t, err, "Unmarshalling")
		require.Equal(t, account, unmarshalled)
	})

	t.Run("Binary", func(t *testing.T) {
		data, err := account.MarshalBinary()
		require.NoError(t, err, "Marshalling")

		unmarshalled := new(TokenAccount)
		err = unmarshalled.UnmarshalBinary(data)
		require.NoError(t, err, "Unmarshalling")
		require.Equal(t, account, unmarshalled)
	})
}
