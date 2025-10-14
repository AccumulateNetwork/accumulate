// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
)

type TestType struct {
	OptionalUrl string `validate:"acc-url"`
	RequiredUrl string `validate:"required,acc-url"`
}

func TestAccUrlValidator(t *testing.T) {
	cases := map[string]struct {
		value *TestType
		ok    bool
	}{
		"None":     {&TestType{}, false},
		"Optional": {&TestType{OptionalUrl: "foo"}, false},
		"Required": {&TestType{RequiredUrl: "foo"}, true},
		"Both":     {&TestType{OptionalUrl: "foo", RequiredUrl: "bar"}, true},
		"Invalid":  {&TestType{RequiredUrl: "https://foo"}, false},
	}

	v, err := NewValidator()
	require.NoError(t, err)

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if c.ok {
				require.NoError(t, v.Struct(c.value))
			} else {
				require.Error(t, v.Struct(c.value))
			}
		})
	}
}

func TestKeyPage_MofN(t *testing.T) {
	kp := new(KeyPage)
	var rh common.RandHash
	for i := 1; i < 11; i++ {
		key := new(KeySpec)
		key.PublicKeyHash = rh.Next()
		key.LastUsedOn = 0
		kp.AddKeySpec(key)
		for j := 1; j < 12; j++ {
			err := kp.SetThreshold(uint64(j))
			require.Truef(t, err == nil || j > i, "error: %v i: %d j %d", err, i, j)
		}
	}
}

func TestKeySpec_MiningFields(t *testing.T) {
	t.Run("Basic", func(t *testing.T) {
		// Test KeySpec with mining fields
		key := &KeySpec{
			PublicKeyHash:    []byte("test-public-key-hash"),
			LastUsedOn:       12345,
			MiningDifficulty: []byte("mining-difficulty-32-bytes-test"),
			MiningExpiry:     67890,
		}

		// Test Copy method
		copied := key.Copy()
		require.Equal(t, key.PublicKeyHash, copied.PublicKeyHash)
		require.Equal(t, key.LastUsedOn, copied.LastUsedOn)
		require.Equal(t, key.MiningDifficulty, copied.MiningDifficulty)
		require.Equal(t, key.MiningExpiry, copied.MiningExpiry)

		// Test Equal method
		require.True(t, key.Equal(copied))

		// Test modification breaks equality
		copied.MiningExpiry = 99999
		require.False(t, key.Equal(copied))
	})

	t.Run("OptionalFields", func(t *testing.T) {
		// Test KeySpec with only required fields (no mining fields)
		key := &KeySpec{
			PublicKeyHash: []byte("test-public-key-hash"),
			LastUsedOn:    12345,
		}

		// Test Copy method with nil mining fields
		copied := key.Copy()
		require.Equal(t, key.PublicKeyHash, copied.PublicKeyHash)
		require.Equal(t, key.LastUsedOn, copied.LastUsedOn)
		require.Nil(t, copied.MiningDifficulty)
		require.Equal(t, uint64(0), copied.MiningExpiry)

		// Test Equal method with nil fields
		require.True(t, key.Equal(copied))
	})

	t.Run("Marshaling", func(t *testing.T) {
		// Test binary marshaling/unmarshaling with mining fields
		original := &KeySpec{
			PublicKeyHash:    []byte("test-public-key-hash"),
			LastUsedOn:       12345,
			MiningDifficulty: []byte("mining-difficulty-32-bytes-test"),
			MiningExpiry:     67890,
		}

		// Binary marshaling
		data, err := original.MarshalBinary()
		require.NoError(t, err)

		// Binary unmarshaling
		decoded := new(KeySpec)
		err = decoded.UnmarshalBinary(data)
		require.NoError(t, err)

		// Verify fields
		require.Equal(t, original.PublicKeyHash, decoded.PublicKeyHash)
		require.Equal(t, original.LastUsedOn, decoded.LastUsedOn)
		require.Equal(t, original.MiningDifficulty, decoded.MiningDifficulty)
		require.Equal(t, original.MiningExpiry, decoded.MiningExpiry)
		require.True(t, original.Equal(decoded))
	})

	t.Run("JSONMarshaling", func(t *testing.T) {
		// Test JSON marshaling/unmarshaling with mining fields
		original := &KeySpec{
			PublicKeyHash:    []byte("test-public-key-hash"),
			LastUsedOn:       12345,
			MiningDifficulty: []byte("mining-difficulty-32-bytes-test"),
			MiningExpiry:     67890,
		}

		// JSON marshaling
		jsonData, err := original.MarshalJSON()
		require.NoError(t, err)

		// JSON unmarshaling
		decoded := new(KeySpec)
		err = decoded.UnmarshalJSON(jsonData)
		require.NoError(t, err)

		// Verify fields
		require.Equal(t, original.PublicKeyHash, decoded.PublicKeyHash)
		require.Equal(t, original.LastUsedOn, decoded.LastUsedOn)
		require.Equal(t, original.MiningDifficulty, decoded.MiningDifficulty)
		require.Equal(t, original.MiningExpiry, decoded.MiningExpiry)
		require.True(t, original.Equal(decoded))
	})

	t.Run("BackwardCompatibility", func(t *testing.T) {
		// Test that KeySpec without mining fields still works
		original := &KeySpec{
			PublicKeyHash: []byte("test-public-key-hash"),
			LastUsedOn:    12345,
		}

		// Binary marshaling
		data, err := original.MarshalBinary()
		require.NoError(t, err)

		// Binary unmarshaling
		decoded := new(KeySpec)
		err = decoded.UnmarshalBinary(data)
		require.NoError(t, err)

		// Verify required fields
		require.Equal(t, original.PublicKeyHash, decoded.PublicKeyHash)
		require.Equal(t, original.LastUsedOn, decoded.LastUsedOn)

		// Verify optional mining fields have zero values
		require.Nil(t, decoded.MiningDifficulty)
		require.Equal(t, uint64(0), decoded.MiningExpiry)
		require.True(t, original.Equal(decoded))
	})
}
