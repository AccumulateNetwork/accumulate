package protocol

import (
	"testing"

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
