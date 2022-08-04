package connections

import (
	"fmt"
)

func errNoHealthyNodes(partitionId string) error {
	return fmt.Errorf("no healthy nodes available (yet) for partition %q", partitionId)
}

func errUnknownPartition(partitionId string) error {
	return fmt.Errorf("unknown partition %q", partitionId)
}

func errNoLocalClient(partitionId string) error {
	return fmt.Errorf("local client not initialized for partition %q", partitionId)
}

func errCreateRPCClient(err error) error {
	return fmt.Errorf("failed to create RPC client: %v", err)
}

func errDNSLookup(host string, err error) error {
	return fmt.Errorf("error doing DNS lookup for %q: %w", host, err)
}
