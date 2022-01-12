package protocol

import (
	"testing"

	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/stretchr/testify/require"
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
		key.PublicKey = rh.Next()
		key.Nonce = 0
		kp.Keys = append(kp.Keys, key)
		for j := 1; j < 12; j++ {
			err := kp.SetThreshold(uint64(j))
			require.Truef(t, err == nil || j > i, "error: %v i: %d j %d", err, i, j)
		}
	}
}
