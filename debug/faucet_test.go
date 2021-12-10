package debug_test

import (
	"testing"

	"github.com/AccumulateNetwork/accumulate/internal/api"
	"github.com/AccumulateNetwork/accumulate/internal/relay"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDebugFaucet(t *testing.T) {
	t.Skip("Skip this test unless you need it to debug something")

	// Networks must be in the same order as they are passed to --relay-to in the node configuration
	clients, err := relay.NewClients(nil, "Arches", "AmericanSamoa", "EastXeons")
	require.NoError(t, err)

	u, err := url.Parse("acc://b5d4ac455c08bedc04a56d8147e9e9c9494c99eb81e9d8c3/ACME")
	require.NoError(t, err)
	expectedNetworkId := u.Routing() % uint64(len(clients))

	found := []int{}
	for i, c := range clients {
		q := api.NewQuery(relay.New(c))
		resp, err := q.QueryByUrl(u.String())
		require.NoError(t, err)
		if resp.Response.Code != 0 {
			require.Contains(t, resp.Response.Info, "not found")
			continue
		}
		found = append(found, i)
	}

	if len(found) == 0 {
		t.Fatal("The account was not found anywhere. You probably need to use the faucet to create it.")
	}
	if len(found) > 1 {
		t.Fatal("The account was found on multiple BVCs... that is not how it should work.")
	}

	assert.Equal(t, expectedNetworkId, uint64(found[0]), "The account is supposed to exist on BVC %d but we actually found it on BVC %d", expectedNetworkId, found[0])
}
