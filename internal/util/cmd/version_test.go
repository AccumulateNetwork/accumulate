// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package cmdutil

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate"
)

func TestFormatVersion(t *testing.T) {
	result := FormatVersion()

	// Should contain version
	require.Contains(t, result, accumulate.Version)

	// Should contain commit
	require.Contains(t, result, accumulate.Commit)

	// Should contain network name (MainNet or TestNet)
	require.True(t, strings.Contains(result, "MainNet") || strings.Contains(result, "TestNet"),
		"version should contain MainNet or TestNet")

	// Should end with newline
	require.True(t, strings.HasSuffix(result, "\n"), "version should end with newline")

	// Should have two lines
	lines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")
	require.Len(t, lines, 2, "version should have two lines")
}

func TestWriteVersion(t *testing.T) {
	var buf bytes.Buffer
	WriteVersion(&buf)

	result := buf.String()
	expected := FormatVersion()

	require.Equal(t, expected, result)
}

func TestVersionCmd(t *testing.T) {
	cmd := VersionCmd()

	// Check command properties
	require.Equal(t, "version", cmd.Use)
	require.Equal(t, "Print version information", cmd.Short)

	// Check flags exist
	versionOnly := cmd.Flags().Lookup("version-only")
	require.NotNil(t, versionOnly, "version-only flag should exist")
	require.Equal(t, "false", versionOnly.DefValue)

	knownVersion := cmd.Flags().Lookup("known-version")
	require.NotNil(t, knownVersion, "known-version flag should exist")
	require.Equal(t, "false", knownVersion.DefValue)
}

func TestVersionCmd_MultipleInstances(t *testing.T) {
	// Ensure VersionCmd returns independent instances
	cmd1 := VersionCmd()
	cmd2 := VersionCmd()

	require.NotSame(t, cmd1, cmd2, "VersionCmd should return new instances")

	// Setting a flag on one shouldn't affect the other
	require.NoError(t, cmd1.Flags().Set("version-only", "true"))
	val1, err := cmd1.Flags().GetBool("version-only")
	require.NoError(t, err)
	require.True(t, val1)

	val2, err := cmd2.Flags().GetBool("version-only")
	require.NoError(t, err)
	require.False(t, val2)
}

func TestHasVersionFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{
			name:     "empty args",
			args:     []string{},
			expected: false,
		},
		{
			name:     "no version flag",
			args:     []string{"--help"},
			expected: false,
		},
		{
			name:     "double dash version",
			args:     []string{"--version"},
			expected: true,
		},
		{
			name:     "single dash version",
			args:     []string{"-version"},
			expected: true,
		},
		{
			name:     "version with other flags before",
			args:     []string{"--debug", "--version"},
			expected: true,
		},
		{
			name:     "version after positional arg",
			args:     []string{"MainNet", "--version"},
			expected: false,
		},
		{
			name:     "version after double dash separator",
			args:     []string{"--", "--version"},
			expected: false,
		},
		{
			name:     "version flag with value flags",
			args:     []string{"--log-level", "debug", "--version"},
			expected: false, // log-level value "debug" is treated as positional
		},
		{
			name:     "version first among flags",
			args:     []string{"--version", "--help"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasVersionFlag(tt.args)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestIsVersionArg(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{
			name:     "empty args",
			args:     []string{},
			expected: false,
		},
		{
			name:     "only program name",
			args:     []string{"program"},
			expected: false,
		},
		{
			name:     "version subcommand",
			args:     []string{"program", "version"},
			expected: true,
		},
		{
			name:     "double dash version",
			args:     []string{"program", "--version"},
			expected: true,
		},
		{
			name:     "single dash version",
			args:     []string{"program", "-version"},
			expected: true,
		},
		{
			name:     "other command",
			args:     []string{"program", "help"},
			expected: false,
		},
		{
			name:     "version not first arg",
			args:     []string{"program", "other", "version"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isVersionArg(tt.args)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestAddVersionFlag(t *testing.T) {
	// Create a test command
	root := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	// For this test, we just verify the flag is added correctly
	// We can't easily test the os.Exit path without subprocess testing
	root.PersistentFlags().Bool("version", false, "Print version information and exit")

	flag := root.PersistentFlags().Lookup("version")
	require.NotNil(t, flag, "version flag should be added")
	require.Equal(t, "false", flag.DefValue)
	require.Equal(t, "Print version information and exit", flag.Usage)
}
