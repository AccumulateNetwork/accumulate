package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "config"), 0777))

	// Create
	cfg := DefaultValidator()
	cfg.SetRoot(dir)
	cfg.Accumulate.Type = BVC
	cfg.Accumulate.API.JSONListenAddress = "api-json-listen"
	cfg.Accumulate.API.RESTListenAddress = "api-rest-listen"

	// Small changes to make Equal happy
	cfg.StateSync.RPCServers = []string{}

	// Store
	require.NoError(t, Store(cfg))

	// Load
	lcfg, err := Load(dir)
	require.NoError(t, err)

	// Should be equal
	require.Equal(t, cfg, lcfg)
}
