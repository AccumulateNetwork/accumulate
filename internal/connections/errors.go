package connections

import (
	"fmt"
)

func errNoHealthyNodes(subnetId string) error {
	return fmt.Errorf("no healthy nodes available (yet) for subnet %q", subnetId)
}

func errUnknownSubnet(subnetId string) error {
	return fmt.Errorf("unknown subnet %q", subnetId)
}

func errNoLocalClient(subnetId string) error {
	return fmt.Errorf("local client not initialized for subnet %q", subnetId)
}

func errInvalidAddress(err error) error {
	return fmt.Errorf("invalid address: %v", err)
}

func errCreateRPCClient(err error) error {
	return fmt.Errorf("failed to create RPC client: %v", err)
}

func errDNSLookup(host string, err error) error {
	return fmt.Errorf("error doing DNS lookup for %q: %w", host, err)
}
