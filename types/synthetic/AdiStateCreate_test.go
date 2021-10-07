package synthetic

import (
	"crypto/sha256"
	"encoding/json"
	"testing"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/stretchr/testify/require"
)

func TestNewAdiStateCreate(t *testing.T) {
	kp := types.CreateKeyPair()
	pubKeyHash := types.Bytes32(sha256.Sum256(kp.PubKey().Bytes()))
	fromAdi := types.String("greenrock")
	toAdi := types.String("redwagon")

	txId := types.Bytes32(sha256.Sum256([]byte("sometxid")))
	original := NewAdiStateCreate(txId[:], &fromAdi, &toAdi, &pubKeyHash)

	t.Run("JSON", func(t *testing.T) {
		data, err := json.Marshal(original)
		require.NoError(t, err, "Marshalling")

		unmarshalled := new(AdiStateCreate)
		err = json.Unmarshal(data, &unmarshalled)
		require.NoError(t, err, "Unmarshalling")
		require.Equal(t, original, unmarshalled)
	})

	t.Run("Binary", func(t *testing.T) {
		data, err := original.MarshalBinary()
		require.NoError(t, err, "Marshalling")

		unmarshalled := new(AdiStateCreate)
		err = unmarshalled.UnmarshalBinary(data)
		require.NoError(t, err, "Unmarshalling")
		require.Equal(t, original, unmarshalled)
	})
}
