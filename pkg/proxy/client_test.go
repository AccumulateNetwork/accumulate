package proxy_test

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/proxy"
	testing2 "gitlab.com/accumulatenetwork/accumulate/pkg/proxy/testing"
)

func TestAccuProxyClient(t *testing.T) {

	// Provided as an example for accuproxy:
	// Steps to secure the proxy.
	client, _, _, _ := testing2.LaunchFakeProxy(t)

	ssr := proxy.PartitionListRequest{}
	ssr.Network = "AccuProxyTest"
	ssr.Sign = true
	PartitionListResp, err := client.GetPartitionList(context.Background(), &ssr)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(PartitionListResp.Partitions), 1)

	d, err := PartitionListResp.PartitionList.MarshalBinary()
	require.NoError(t, err)
	hash := sha256.Sum256(d)
	require.True(t, PartitionListResp.Signature.Verify(nil, hash[:]))

	scr := proxy.SeedCountRequest{}
	scr.Network = "AccuProxyTest"
	scr.Sign = true
	scr.Partition = PartitionListResp.Partitions[0]
	seedCountResp, err := client.GetSeedCount(context.Background(), &scr)
	require.NoError(t, err)
	d, err = seedCountResp.SeedCount.MarshalBinary()
	require.NoError(t, err)
	hash = sha256.Sum256(d)
	require.True(t, seedCountResp.Signature.Verify(nil, hash[:]))

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
	require.True(t, seedListResp.Signature.Verify(nil, hash[:]))

	require.Equal(t, len(seedListResp.Addresses), len(testing2.Nodes))

	ncr := proxy.NetworkConfigRequest{}
	ncr.Sign = true
	ncr.Network = "AccuProxyTest"
	ncResp, err := client.GetNetworkConfig(context.Background(), &ncr)
	require.NoError(t, err)
	require.True(t, ncResp.NetworkState.Network.Equal(&testing2.Network))
	d, err = ncResp.NetworkState.MarshalBinary()
	require.NoError(t, err)
	hash = sha256.Sum256(d)
	require.True(t, ncResp.Signature.Verify(nil, hash[:]))
}
