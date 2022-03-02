package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/url"
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
		"Has query":               {URL{Authority: "foo", Query: "bar"}, "query is not empty"},
		"Has fragment":            {URL{Authority: "foo", Fragment: "bar"}, "fragment is not empty"},
		"Identity has dot":        {URL{Authority: "foo.bar"}, "identity contains dot(s)"},
		"Identity has underscore": {URL{Authority: "foo_bar"}, "illegal character '_'"},
		"Identity has space":      {URL{Authority: "foo bar"}, "illegal character ' '"},
		"Looks like lite acct lc": {URL{Authority: strings.ToLower(randHex(24))}, "identity could be a lite token account key"},
		"Looks like lite acct uc": {URL{Authority: strings.ToUpper(randHex(24))}, "identity could be a lite token account key"},
	}

	for name, str := range good {
		t.Run(name, func(t *testing.T) {
			fmt.Println(str)
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

func TestLiteAddress(t *testing.T) {
	TokenURLs := map[string]string{
		"good1": "RedWaggon/Wheels",
		"good2": "BlueBall/Footballs",
		"good3": "ACME",
		"bad1":  "RedWaggon",
		"bad2":  "Red_Waggon/Wheels",
		"bad3":  "BlueBall.com/Footballs",
	}
	for name, str := range TokenURLs {
		t.Run(name, func(t *testing.T) {
			publicKey := sha256.Sum256([]byte(name))
			url, err := LiteTokenAddress(publicKey[:], str)
			if name[:4] == "good" {
				require.NoError(t, err, "%s should be valid", str)
				fmt.Println(url.String())
			} else {
				require.Errorf(t, err, " %s should be invalid", str)
			}
		})
	}
}

func TestParseLiteTokenAddress(t *testing.T) {
	fakeKey := make([]byte, 32)
	fakeHash := sha256.Sum256(fakeKey)
	addr, err := LiteTokenAddress(fakeKey, "-/-")
	require.NoError(t, err)
	addr = addr.RootIdentity()

	tests := []string{
		"ACME",
		"foo/tokens",
	}
	for _, test := range tests {
		t.Run(test, func(t *testing.T) {
			joinPath := addr.JoinPath("/" + test)
			key, tok, err := ParseLiteTokenAddress(joinPath)
			require.NoError(t, err)
			require.Equal(t, fakeHash[:20], key)
			require.Equal(t, "acc://"+test, tok.String())
		})
	}
}
