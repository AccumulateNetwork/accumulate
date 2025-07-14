// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package blocks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/cometbft"
)

// Response is a generic API response.
//
// TODO: This is a temporary solution. The API v3 package should provide a
// generic response type.
type Response[T any] struct {
	Value T            `json:"value,omitempty"`
	Error *errors.Error `json:"error,omitempty"`
}

// GenesisAuthorityProvider provides authorities from the genesis document of a CometBFT network.

// Ensure GenesisAuthorityProvider implements the AuthorityProvider interface.
var _ AuthorityProvider = (*GenesisAuthorityProvider)(nil)

// GenesisAuthorityProvider provides authorities from the genesis document of a CometBFT network. by
// querying the /genesis endpoint of a node.
//
// TODO: This is not robust. What if the node is malicious?
type GenesisAuthorityProvider struct {
	Client *http.Client
	Host   string
}

// NewGenesisAuthorityProvider creates a new GenesisAuthorityProvider.
func NewGenesisAuthorityProvider(client *http.Client, host string) *GenesisAuthorityProvider {
	return &GenesisAuthorityProvider{
		Client: client,
		Host:   host,
	}
}

// GetAuthorities implements AuthorityProvider.
func (p *GenesisAuthorityProvider) GetAuthorities(ctx context.Context) (map[[32]byte]bool, uint64, error) {
	// Construct and execute the request
	req, err := http.NewRequestWithContext(ctx, "GET", p.Host+"/v3/genesis", nil)
	if err != nil {
		return nil, 0, fmt.Errorf("construct request: %w", err)
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read the body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("read body: %w", err)
	}

	// Unmarshal the response
	var genResp Response[*cometbft.GenesisDoc]
	err = json.Unmarshal(body, &genResp)

	// Check for errors
	if err != nil {
		return nil, 0, fmt.Errorf("unmarshal response: %w", err)
	} else if genResp.Error != nil {
		return nil, 0, genResp.Error
	} else if genResp.Value == nil {
		return nil, 0, fmt.Errorf("genesis document is missing from the response")
	} else if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("bad status: %s", resp.Status)
	}

	// Extract the authorities and calculate the threshold
	auths := make(map[[32]byte]bool)
	var totalPower int64
	for _, val := range genResp.Value.Validators {
		// The key is the public key
		var key [32]byte
		copy(key[:], val.PubKey)

		auths[key] = true
		totalPower += val.Power
	}

	// The threshold is floor(2/3*P)+1
	threshold := totalPower*2/3 + 1
	return auths, uint64(threshold), nil
}
