// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package connections

import (
	"errors"
	"fmt"
)

var ErrNoDirect = errors.New("no direct connection")

func errNoDirect(partition string) error {
	return fmt.Errorf("%w for %s", ErrNoDirect, partition)
}
func errNoHealthyNodes(partitionId string) error {
	return fmt.Errorf("no healthy nodes available (yet) for partition %q", partitionId)
}

func errUnknownPartition(partitionId string) error {
	return fmt.Errorf("unknown partition %q", partitionId)
}

func errNoLocalClient(partitionId string) error {
	return fmt.Errorf("local client not initialized for partition %q", partitionId)
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
