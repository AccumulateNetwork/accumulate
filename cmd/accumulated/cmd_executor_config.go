// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/bits"
	"net/http"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

var cmdExecutorConfig = &cobra.Command{
	Use:   "executor-config",
	Short: "Manage executor shard configuration",
	Long: `View and manage the executor shard count.

The shard count controls how many parallel execution shards the node uses.
It must be a power of 2 between 4 and 256.

Changes take effect at the next block boundary.`,
}

var cmdExecutorConfigGet = &cobra.Command{
	Use:   "get",
	Short: "Show current shard count",
	Long:  `Query a running node for its current executor shard configuration.`,
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runExecutorConfigGet(cmd, args)
		printOutput(cmd, out, err)
	},
}

var cmdExecutorConfigSet = &cobra.Command{
	Use:   "set <count>",
	Short: "Update shard count",
	Long: `Set the executor shard count on a running node. The value must be a power of 2
between 4 and 256 (valid values: 4, 8, 16, 32, 64, 128, 256).

The change takes effect at the next block boundary.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runExecutorConfigSet(cmd, args)
		printOutput(cmd, out, err)
	},
}

var cmdExecutorConfigValidate = &cobra.Command{
	Use:   "validate <count>",
	Short: "Check if a shard count is valid",
	Long:  `Validate whether a given shard count is acceptable (power of 2, range 4-256).`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runExecutorConfigValidate(cmd, args)
		printOutput(cmd, out, err)
	},
}

var flagExecutorConfig struct {
	JSON     bool
	Endpoint string
}

func init() {
	cmdMain.AddCommand(cmdExecutorConfig)
	cmdExecutorConfig.AddCommand(cmdExecutorConfigGet)
	cmdExecutorConfig.AddCommand(cmdExecutorConfigSet)
	cmdExecutorConfig.AddCommand(cmdExecutorConfigValidate)

	cmdExecutorConfig.PersistentFlags().BoolVar(&flagExecutorConfig.JSON, "json", false, "Output in JSON format")
	cmdExecutorConfig.PersistentFlags().StringVarP(&flagExecutorConfig.Endpoint, "endpoint", "e", "http://localhost:26660", "Node API endpoint")
}

func runExecutorConfigGet(_ *cobra.Command, _ []string) (string, error) {
	url := flagExecutorConfig.Endpoint + "/executor/config"
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to connect to node: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("node returned error: %s", string(body))
	}

	if flagExecutorConfig.JSON {
		// Pretty-print the JSON
		var buf bytes.Buffer
		if err := json.Indent(&buf, body, "", "  "); err != nil {
			return string(body), nil
		}
		return buf.String(), nil
	}

	var result struct {
		ShardCount uint64 `json:"shardCount"`
		Depth      int    `json:"depth"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return fmt.Sprintf(`Executor Shard Configuration
============================
Shard Count: %d
Depth:       %d
Valid Range: 4, 8, 16, 32, 64, 128, 256`, result.ShardCount, result.Depth), nil
}

func runExecutorConfigSet(_ *cobra.Command, args []string) (string, error) {
	count, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid shard count %q: %w", args[0], err)
	}

	// Validate locally first for immediate feedback.
	if err := database.ValidateShardCount(count); err != nil {
		return "", err
	}

	url := flagExecutorConfig.Endpoint + "/executor/config"
	client := &http.Client{Timeout: 10 * time.Second}

	reqBody, _ := json.Marshal(map[string]uint64{"shardCount": count})
	resp, err := client.Post(url, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to connect to node: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("node returned error: %s", string(body))
	}

	depth := bits.TrailingZeros64(count)
	if flagExecutorConfig.JSON {
		data, err := json.MarshalIndent(map[string]interface{}{
			"shardCount": count,
			"depth":      depth,
			"status":     "updated",
		}, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	return fmt.Sprintf("Shard count updated to %d (depth: %d)", count, depth), nil
}

func runExecutorConfigValidate(_ *cobra.Command, args []string) (string, error) {
	count, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid number %q: %w", args[0], err)
	}

	validErr := database.ValidateShardCount(count)

	if flagExecutorConfig.JSON {
		result := map[string]interface{}{
			"shardCount": count,
			"valid":      validErr == nil,
		}
		if validErr != nil {
			result["error"] = validErr.Error()
		} else {
			result["depth"] = bits.TrailingZeros64(count)
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	if validErr != nil {
		return fmt.Sprintf("Invalid: %s", validErr.Error()), nil
	}

	depth := bits.TrailingZeros64(count)
	return fmt.Sprintf("Valid: shard count %d (depth: %d)", count, depth), nil
}
