//go:build !testnet
// +build !testnet

package load_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

func NewLoadTestContext(config LoadConfig) *LoadTestContext {
	endpoint, err := FindDevnetEndpoint()
	if err != nil {
		return nil
	}
	
	client := jsonrpc.NewClient(endpoint)
	client.Client.Timeout = DefaultTimeout
	
	ctx := context.Background()
	
	status, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return nil
	}
	
	seed := GenerateSeed()
	
	testCtx = &LoadTestContext{
		Client:  client,
		Context: ctx,
		Seed:    seed,
		Oracle:  status.Oracle.Price,
		Config:  config,
	}
	
	return testCtx
}

func SetupClient(endpoint string) (*jsonrpc.Client, error) {
	client := jsonrpc.NewClient(endpoint)
	client.Client.Timeout = DefaultTimeout
	
	ctx := context.Background()
	_, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to endpoint: %w", err)
	}
	
	return client, nil
}

func FindDevnetEndpointLocal() string {
	ports := []string{"26660", "36660", "46660"}
	
	for _, port := range ports {
		endpoint := fmt.Sprintf("http://127.0.0.1:%s/v3", port)
		
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%s", port), 2*time.Second)
		if err != nil {
			continue
		}
		conn.Close()
		
		client := jsonrpc.NewClient(endpoint)
		client.Client.Timeout = 5 * time.Second
		
		_, err = client.NetworkStatus(context.Background(), api.NetworkStatusOptions{})
		if err == nil {
			return endpoint
		}
	}
	
	return ""
}

func GetOracle(client *jsonrpc.Client) (uint64, error) {
	ctx := context.Background()
	status, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return 0, err
	}
	
	if status.Oracle.Price == 0 {
		return 5000, nil
	}
	
	return status.Oracle.Price, nil
}

func GenerateSeed() [32]byte {
	timestamp := time.Now().UnixNano()
	return sha256.Sum256([]byte(fmt.Sprintf("%d", timestamp)))
}