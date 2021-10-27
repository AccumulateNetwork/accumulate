package protocol

import (
	"encoding/hex"
	"strings"
	"testing"

	. "github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/rand"
)

func TestIsValidAdiUrl(t *testing.T) {
	rand := rand.New(rand.NewSource(0))
	randHex := func(n int) string {
		b := make([]byte, n)
		n, err := rand.Read(b)
		require.Equal(t, len(b), n)
		require.NoError(t, err)
		return hex.EncodeToString(b)
	}

	good := map[string]string{
		"Simple":            "foo",
		"Identity has dash": "foo-bar",
	}
	bad := map[string]struct {
		URL URL
		err string
	}{
		"Invalid UTF-8":           {URL{Authority: "\xF1"}, "not valid UTF-8"},
		"Has port":                {URL{Authority: "foo:123"}, "identity has a port number"},
		"Empty identity":          {URL{}, "identity is empty"},
		"Has path":                {URL{Authority: "foo", Path: "bar"}, "path is not empty"},
		"Has query":               {URL{Authority: "foo", Query: "bar"}, "query is not empty"},
		"Has fragment":            {URL{Authority: "foo", Fragment: "bar"}, "fragment is not empty"},
		"Identity has dot":        {URL{Authority: "foo.bar"}, "identity contains dot(s)"},
		"Identity has underscore": {URL{Authority: "foo_bar"}, "illegal character '_'"},
		"Identity has space":      {URL{Authority: "foo bar"}, "illegal character ' '"},
		"Looks like lite acct lc": {URL{Authority: strings.ToLower(randHex(24))}, "identity could be a lite account key"},
		"Looks like lite acct uc": {URL{Authority: strings.ToUpper(randHex(24))}, "identity could be a lite account key"},
	}

	for name, str := range good {
		t.Run(name, func(t *testing.T) {
			u, err := Parse(str)
			require.NoError(t, err)
			require.NoError(t, IsValidAdiUrl(u))
		})
	}

	for name, c := range bad {
		t.Run(name, func(t *testing.T) {
			require.EqualError(t, IsValidAdiUrl(&c.URL), c.err)
		})
	}
}
