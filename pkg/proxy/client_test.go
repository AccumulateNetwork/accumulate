package proxy_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/proxy"
	testing2 "gitlab.com/accumulatenetwork/accumulate/pkg/proxy/testing"
)

func createProxyAccounts(node, nodeKey ed25519.PrivateKey) ed25519.PrivateKey {

	return ed25519.NewKeyFromSeed(nil)
}

func TestAccuProxyClient(t *testing.T) {

	// Provided as an example for accuproxy:
	// Steps to secure the proxy.

	// Create the lite addresses and one account
	//sim := simulator.New(t, 1)
	//sim.InitFromGenesis()

	//accClient := testing2.LaunchBasicDevnet(t, 34000)
	//
	//dnNetwork := protocol.DnUrl().JoinPath(protocol.Network)
	//q := query.RequestByUrl{Url: dnNetwork}
	//sim.Query(protocol.DnUrl(), &q, true) //don't really need to prove, but the converse is true, need to prove the dn.acme/network account

	//createProxyAccounts(d.,key)

	client := testing2.LaunchFakeProxy(t)

	ssr := proxy.PartitionListRequest{}
	ssr.Network = "AccuProxyTest"
	ssr.Sign = true
	PartitionListResp, err := client.GetPartitionList(context.Background(), &ssr)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(PartitionListResp.Partitions), 1)

	d, err := PartitionListResp.PartitionList.MarshalBinary()
	require.NoError(t, err)
	hash := sha256.Sum256(d)
	require.True(t, PartitionListResp.Signature.Verify(hash[:]))

	scr := proxy.SeedCountRequest{}
	scr.Network = "AccuProxyTest"
	scr.Sign = true
	scr.Partition = PartitionListResp.Partitions[0]
	seedCountResp, err := client.GetSeedCount(context.Background(), &scr)
	require.NoError(t, err)
	d, err = seedCountResp.SeedCount.MarshalBinary()
	require.NoError(t, err)
	hash = sha256.Sum256(d)
	require.True(t, seedCountResp.Signature.Verify(hash[:]))

	slr := proxy.SeedListRequest{}
	slr.Sign = true
	slr.Network = "AccuProxyTest"
	slr.Partition = PartitionListResp.Partitions[0]
	slr.Count = seedCountResp.Count
	seedListResp, err := client.GetSeedList(context.Background(), &slr)
	require.NoError(t, err)

	d, err = seedListResp.SeedList.MarshalBinary()
	require.NoError(t, err)
	hash = sha256.Sum256(d)
	require.True(t, seedListResp.Signature.Verify(hash[:]))

	require.Equal(t, len(seedListResp.Addresses), len(testing2.Nodes))

	ncr := proxy.NetworkConfigRequest{}
	ncr.Sign = true
	ncr.Network = "AccuProxyTest"
	ncResp, err := client.GetNetworkConfig(context.Background(), &ncr)
	require.NoError(t, err)
	require.True(t, ncResp.Network.Equal(&testing2.Network))
	d, err = ncResp.Network.MarshalBinary()
	require.NoError(t, err)
	hash = sha256.Sum256(d)
	require.True(t, ncResp.Signature.Verify(hash[:]))

}
