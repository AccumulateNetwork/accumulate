package types

import (
	"testing"
)

func TestValidator(t *testing.T) {
	v1 := newVal(t, vally1, pk1)
	v2 := newVal(t, vally1, pk1)

	require.Equal(t, v1, v2)
	v2 = newVal(t, vally2, pk2)

	require.Equal(t, v1, v2)

}

func newVal(t *testing.T, operator string, pubKey ed25519.PubKey) protocol.ValidatorType {
	v, err := NewValidator(operator, pubKey, big.NewInt(0), types.BondedStatusBonded, 0, "", "", "", "")
	require.NoError(t, err)
	return v
}