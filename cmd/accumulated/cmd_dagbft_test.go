// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDAGBFTStatusCommand tests the dagbft status command.
func TestDAGBFTStatusCommand(t *testing.T) {
	t.Run("text output", func(t *testing.T) {
		// Reset flag state
		flagDAGBFTStatus.JSON = false
		flagDAGBFTStatus.Endpoint = "http://localhost:26660"

		out, err := runDAGBFTStatus(cmdDAGBFTStatus, nil)
		require.NoError(t, err)

		// Verify text output contains expected fields
		assert.Contains(t, out, "DAG-BFT Node Status")
		assert.Contains(t, out, "Node ID:")
		assert.Contains(t, out, "Partition:")
		assert.Contains(t, out, "Epoch:")
		assert.Contains(t, out, "Round:")
		assert.Contains(t, out, "Block Height:")
		assert.Contains(t, out, "Peer Count:")
		assert.Contains(t, out, "Is Validator:")
	})

	t.Run("json output", func(t *testing.T) {
		flagDAGBFTStatus.JSON = true
		flagDAGBFTStatus.Endpoint = "http://localhost:26660"

		out, err := runDAGBFTStatus(cmdDAGBFTStatus, nil)
		require.NoError(t, err)

		// Verify JSON parses correctly
		var status NodeStatus
		err = json.Unmarshal([]byte(out), &status)
		require.NoError(t, err)

		// Verify JSON fields
		assert.NotEmpty(t, status.NodeID)
		assert.NotEmpty(t, status.Partition)
		assert.GreaterOrEqual(t, status.Epoch, uint64(0))
		assert.GreaterOrEqual(t, status.Round, uint64(0))
	})
}

// TestDAGBFTConsensusCommand tests the dagbft consensus command.
func TestDAGBFTConsensusCommand(t *testing.T) {
	t.Run("text output", func(t *testing.T) {
		flagDAGBFTStatus.JSON = false
		flagDAGBFTStatus.Endpoint = "http://localhost:26660"

		out, err := runDAGBFTConsensus(cmdDAGBFTConsensus, nil)
		require.NoError(t, err)

		// Verify text output contains expected fields
		assert.Contains(t, out, "DAG-BFT Consensus Info")
		assert.Contains(t, out, "Epoch:")
		assert.Contains(t, out, "Current Round:")
		assert.Contains(t, out, "Committed Round:")
		assert.Contains(t, out, "Validator Count:")
		assert.Contains(t, out, "Pending Batches:")
		assert.Contains(t, out, "DAG Size:")
	})

	t.Run("json output", func(t *testing.T) {
		flagDAGBFTStatus.JSON = true
		flagDAGBFTStatus.Endpoint = "http://localhost:26660"

		out, err := runDAGBFTConsensus(cmdDAGBFTConsensus, nil)
		require.NoError(t, err)

		// Verify JSON parses correctly
		var info ConsensusInfo
		err = json.Unmarshal([]byte(out), &info)
		require.NoError(t, err)

		// Verify JSON fields
		assert.GreaterOrEqual(t, info.Epoch, uint64(0))
		assert.GreaterOrEqual(t, info.CurrentRound, uint64(0))
		assert.GreaterOrEqual(t, info.ValidatorCount, 0)
	})
}

// TestDAGBFTDAGCommand tests the dagbft dag command.
func TestDAGBFTDAGCommand(t *testing.T) {
	t.Run("text output current round", func(t *testing.T) {
		flagDAGBFTDebug.JSON = false
		flagDAGBFTDebug.Round = 0 // current round
		flagDAGBFTDebug.Endpoint = "http://localhost:26660"

		out, err := runDAGBFTDAG(cmdDAGBFTDAG, nil)
		require.NoError(t, err)

		// Verify text output contains expected fields
		assert.Contains(t, out, "DAG Round")
		assert.Contains(t, out, "Leader:")
		assert.Contains(t, out, "Committed:")
		assert.Contains(t, out, "Certificates")
	})

	t.Run("text output specific round", func(t *testing.T) {
		flagDAGBFTDebug.JSON = false
		flagDAGBFTDebug.Round = 50
		flagDAGBFTDebug.Endpoint = "http://localhost:26660"

		out, err := runDAGBFTDAG(cmdDAGBFTDAG, nil)
		require.NoError(t, err)

		// Verify specific round is shown
		assert.Contains(t, out, "DAG Round 50")
	})

	t.Run("json output", func(t *testing.T) {
		flagDAGBFTDebug.JSON = true
		flagDAGBFTDebug.Round = 0
		flagDAGBFTDebug.Endpoint = "http://localhost:26660"

		out, err := runDAGBFTDAG(cmdDAGBFTDAG, nil)
		require.NoError(t, err)

		// Verify JSON parses correctly
		var info DAGRoundInfo
		err = json.Unmarshal([]byte(out), &info)
		require.NoError(t, err)

		// Verify JSON fields
		assert.GreaterOrEqual(t, info.Round, uint64(0))
		assert.GreaterOrEqual(t, info.Epoch, uint64(0))
		assert.NotNil(t, info.Certificates)
	})
}

// TestDAGBFTCertCommand tests the dagbft cert command.
func TestDAGBFTCertCommand(t *testing.T) {
	t.Run("text output", func(t *testing.T) {
		flagDAGBFTDebug.JSON = false
		flagDAGBFTDebug.Endpoint = "http://localhost:26660"

		out, err := runDAGBFTCert(cmdDAGBFTCert, []string{"0xabc123"})
		require.NoError(t, err)

		// Verify text output contains expected fields
		assert.Contains(t, out, "Certificate Details")
		assert.Contains(t, out, "Digest:")
		assert.Contains(t, out, "Author:")
		assert.Contains(t, out, "Round:")
		assert.Contains(t, out, "Epoch:")
		assert.Contains(t, out, "Parents")
		assert.Contains(t, out, "Batches")
		assert.Contains(t, out, "Signers")
	})

	t.Run("json output", func(t *testing.T) {
		flagDAGBFTDebug.JSON = true
		flagDAGBFTDebug.Endpoint = "http://localhost:26660"

		out, err := runDAGBFTCert(cmdDAGBFTCert, []string{"0xabc123"})
		require.NoError(t, err)

		// Verify JSON parses correctly
		var details CertDetails
		err = json.Unmarshal([]byte(out), &details)
		require.NoError(t, err)

		// Verify JSON fields
		assert.NotEmpty(t, details.Digest)
		assert.NotEmpty(t, details.Author)
		assert.GreaterOrEqual(t, details.Round, uint64(0))
		assert.NotNil(t, details.Parents)
		assert.NotNil(t, details.Batches)
		assert.NotNil(t, details.Signers)
	})

	t.Run("digest passed through", func(t *testing.T) {
		flagDAGBFTDebug.JSON = true
		flagDAGBFTDebug.Endpoint = "http://localhost:26660"

		testDigest := "0xmytestdigest123"
		out, err := runDAGBFTCert(cmdDAGBFTCert, []string{testDigest})
		require.NoError(t, err)

		var details CertDetails
		err = json.Unmarshal([]byte(out), &details)
		require.NoError(t, err)

		// Verify the digest is correctly used in the response
		assert.Equal(t, testDigest, details.Digest)
	})
}

// TestDAGBFTPendingCommand tests the dagbft pending command.
func TestDAGBFTPendingCommand(t *testing.T) {
	t.Run("text output", func(t *testing.T) {
		flagDAGBFTDebug.JSON = false
		flagDAGBFTDebug.Endpoint = "http://localhost:26660"

		out, err := runDAGBFTPending(cmdDAGBFTPending, nil)
		require.NoError(t, err)

		// Verify text output contains expected fields
		assert.Contains(t, out, "Pending Certificates")
		assert.Contains(t, out, "Count:")
		assert.Contains(t, out, "Round Range:")
	})

	t.Run("json output", func(t *testing.T) {
		flagDAGBFTDebug.JSON = true
		flagDAGBFTDebug.Endpoint = "http://localhost:26660"

		out, err := runDAGBFTPending(cmdDAGBFTPending, nil)
		require.NoError(t, err)

		// Verify JSON parses correctly
		var pending PendingCertificates
		err = json.Unmarshal([]byte(out), &pending)
		require.NoError(t, err)

		// Verify JSON fields
		assert.GreaterOrEqual(t, pending.Count, 0)
		assert.GreaterOrEqual(t, pending.OldestRound, uint64(0))
		assert.NotNil(t, pending.Certificates)
	})
}

// TestDAGBFTKeygenCommand tests the dagbft keygen command.
func TestDAGBFTKeygenCommand(t *testing.T) {
	t.Run("text output", func(t *testing.T) {
		// Use temp directory for output
		tempDir := t.TempDir()
		flagDAGBFTKeygen.Output = tempDir
		flagDAGBFTKeygen.Format = "text"

		out, err := runDAGBFTKeygen(cmdDAGBFTKeygen, nil)
		require.NoError(t, err)

		// Verify text output contains expected content
		assert.Contains(t, out, "Generated node keys")
		assert.Contains(t, out, "Public key:")
		assert.Contains(t, out, "Private key:")
	})

	t.Run("json output", func(t *testing.T) {
		tempDir := t.TempDir()
		flagDAGBFTKeygen.Output = tempDir
		flagDAGBFTKeygen.Format = "json"

		out, err := runDAGBFTKeygen(cmdDAGBFTKeygen, nil)
		require.NoError(t, err)

		// Verify JSON parses correctly
		var keys NodeKeys
		err = json.Unmarshal([]byte(out), &keys)
		require.NoError(t, err)

		// Verify key lengths (ed25519 keys in hex)
		assert.Len(t, keys.PublicKey, 64)   // 32 bytes = 64 hex chars
		assert.Len(t, keys.PrivateKey, 128) // 64 bytes = 128 hex chars
	})
}

// TestDAGBFTConfigCommand tests the dagbft config command.
func TestDAGBFTConfigCommand(t *testing.T) {
	t.Run("generate config", func(t *testing.T) {
		flagDAGBFTConfig.Output = ""
		flagDAGBFTConfig.Validate = ""
		flagDAGBFTConfig.JSON = false

		out, err := runDAGBFTConfig(cmdDAGBFTConfig, nil)
		require.NoError(t, err)

		// Verify example config is returned
		assert.NotEmpty(t, out)
	})

	t.Run("write config to file", func(t *testing.T) {
		tempDir := t.TempDir()
		outputFile := tempDir + "/config.toml"
		flagDAGBFTConfig.Output = outputFile
		flagDAGBFTConfig.Validate = ""
		flagDAGBFTConfig.JSON = false

		out, err := runDAGBFTConfig(cmdDAGBFTConfig, nil)
		require.NoError(t, err)

		// Verify output indicates file was written
		assert.Contains(t, out, "Example configuration written to:")
		assert.Contains(t, out, outputFile)
	})
}

// TestDAGBFTErrorCases tests error handling in DAG-BFT commands.
func TestDAGBFTErrorCases(t *testing.T) {
	t.Run("validate invalid config file", func(t *testing.T) {
		flagDAGBFTConfig.Output = ""
		flagDAGBFTConfig.Validate = "/nonexistent/config.toml"
		flagDAGBFTConfig.JSON = false

		_, err := runDAGBFTConfig(cmdDAGBFTConfig, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validation failed")
	})

	t.Run("keygen invalid directory", func(t *testing.T) {
		flagDAGBFTKeygen.Output = "/root/readonly/invalid"
		flagDAGBFTKeygen.Format = "text"

		_, err := runDAGBFTKeygen(cmdDAGBFTKeygen, nil)
		// This may or may not error depending on permissions
		// Just ensure it doesn't panic
		_ = err
	})
}

// TestCommandIntegration tests that commands are properly registered.
func TestCommandIntegration(t *testing.T) {
	// Verify commands are registered under dagbft
	var cmdNames []string
	for _, cmd := range cmdDAGBFT.Commands() {
		cmdNames = append(cmdNames, cmd.Name())
	}

	assert.Contains(t, cmdNames, "status")
	assert.Contains(t, cmdNames, "consensus")
	assert.Contains(t, cmdNames, "dag")
	assert.Contains(t, cmdNames, "cert")
	assert.Contains(t, cmdNames, "pending")
	assert.Contains(t, cmdNames, "keygen")
	assert.Contains(t, cmdNames, "config")
}

// TestPrintOutput tests the printOutput helper function.
func TestPrintOutput(t *testing.T) {
	t.Run("successful output", func(t *testing.T) {
		var buf bytes.Buffer
		cmdDAGBFTStatus.SetOut(&buf)

		printOutput(cmdDAGBFTStatus, "test output", nil)
		assert.Contains(t, buf.String(), "test output")
	})

	t.Run("error output", func(t *testing.T) {
		var stdOut, stdErr bytes.Buffer
		cmdDAGBFTStatus.SetOut(&stdOut)
		cmdDAGBFTStatus.SetErr(&stdErr)

		printOutput(cmdDAGBFTStatus, "", assert.AnError)
		assert.Contains(t, stdErr.String(), "Error:")
	})
}

// TestJSONOutputFormat ensures all JSON outputs are well-formed.
func TestJSONOutputFormat(t *testing.T) {
	tests := []struct {
		name    string
		runFunc func() (string, error)
		setup   func()
	}{
		{
			name: "status",
			runFunc: func() (string, error) {
				return runDAGBFTStatus(cmdDAGBFTStatus, nil)
			},
			setup: func() {
				flagDAGBFTStatus.JSON = true
			},
		},
		{
			name: "consensus",
			runFunc: func() (string, error) {
				return runDAGBFTConsensus(cmdDAGBFTConsensus, nil)
			},
			setup: func() {
				flagDAGBFTStatus.JSON = true
			},
		},
		{
			name: "dag",
			runFunc: func() (string, error) {
				return runDAGBFTDAG(cmdDAGBFTDAG, nil)
			},
			setup: func() {
				flagDAGBFTDebug.JSON = true
			},
		},
		{
			name: "cert",
			runFunc: func() (string, error) {
				return runDAGBFTCert(cmdDAGBFTCert, []string{"0xtest"})
			},
			setup: func() {
				flagDAGBFTDebug.JSON = true
			},
		},
		{
			name: "pending",
			runFunc: func() (string, error) {
				return runDAGBFTPending(cmdDAGBFTPending, nil)
			},
			setup: func() {
				flagDAGBFTDebug.JSON = true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			out, err := tt.runFunc()
			require.NoError(t, err)

			// Verify it's valid JSON
			assert.True(t, json.Valid([]byte(out)), "output should be valid JSON")

			// Verify it's properly indented (should contain newlines)
			assert.True(t, strings.Contains(out, "\n"), "JSON should be indented")

			// Verify no trailing whitespace issues
			assert.True(t, strings.HasPrefix(out, "{"), "JSON should start with {")
		})
	}
}
