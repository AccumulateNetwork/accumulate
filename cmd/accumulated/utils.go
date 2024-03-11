// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

func getVersion(client *client.Client) (*api.VersionResponse, error) {
	resp, err := client.Version(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get version, %v", err)
	}

	data, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to get version, %v", err)
	}

	version := new(api.VersionResponse)
	err = json.Unmarshal(data, version)
	if err != nil {
		return nil, fmt.Errorf("failed to get version, %v", err)
	}
	return version, err
}

var DidError error

func printOutput(cmd *cobra.Command, out string, err error) {
	if err == nil {
		cmd.Println(out)
		return
	}

	DidError = err
	if out != "" {
		cmd.Println(out)
	}
	cmd.PrintErrf("Error: %v\n", err)
}
