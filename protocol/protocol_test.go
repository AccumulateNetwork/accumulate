package protocol

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/url"
)

func TestIsValidAdiUrl(t *testing.T) {
	good := map[string]string{
		"Simple":            "foo.acme",
		"Identity has dash": "foo-bar.acme",
	}
	bad := map[string]struct {
		URL URL
		err string
	}{
		"Invalid UTF-8":           {URL{Authority: "\xF1.acme"}, "not valid UTF-8"},
		"Has port":                {URL{Authority: "foo.acme:123"}, "identity has a port number"},
		"Empty identity":          {URL{}, "identity is empty"},
		"Missing TLD":             {URL{Authority: "foo"}, "identity must end in .acme"},
		"Has query":               {URL{Authority: "foo.acme", Query: "bar"}, "query is not empty"},
		"Has fragment":            {URL{Authority: "foo.acme", Fragment: "bar"}, "fragment is not empty"},
		"Identity has dot":        {URL{Authority: "foo.bar.acme"}, "identity contains a subdomain"},
		"Identity has underscore": {URL{Authority: "foo_bar.acme"}, "illegal character '_'"},
		"Identity has space":      {URL{Authority: "foo bar.acme"}, "illegal character ' '"},
		"Empty identity with TLD": {URL{Authority: ".acme"}, "identity is empty"},
	}

	for name, str := range good {
		t.Run(name, func(t *testing.T) {
			u, err := Parse(str)
			require.NoError(t, err)
			require.NoError(t, IsValidAdiUrl(u, false))
		})
	}

	for name, c := range bad {
		t.Run(name, func(t *testing.T) {
			require.EqualError(t, IsValidAdiUrl(&c.URL, false), c.err)
		})
	}
}

func TestLiteAddress(t *testing.T) {
	TokenURLs := map[string]string{
		"good1": "RedWaggon.acme/Wheels",
		"good2": "BlueBall.acme/Footballs",
		"good3": "ACME",
		"bad1":  "RedWaggon.acme",
		"bad2":  "Red_Waggon.acme/Wheels",
		"bad3":  "BlueBall.com/Footballs",
	}
	for name, str := range TokenURLs {
		t.Run(name, func(t *testing.T) {
			publicKey := sha256.Sum256([]byte(name))
			_, err := LiteTokenAddress(publicKey[:], str, SignatureTypeED25519)
			if name[:4] == "good" {
				require.NoError(t, err, "%s should be valid", str)
			} else {
				require.Errorf(t, err, " %s should be invalid", str)
			}
		})
	}
}

func TestParseLiteTokenAddress(t *testing.T) {
	fakeKey := make([]byte, 32)
	fakeHash := sha256.Sum256(fakeKey)
	addr, err := LiteTokenAddress(fakeKey, "-.acme/-", SignatureTypeED25519)
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
