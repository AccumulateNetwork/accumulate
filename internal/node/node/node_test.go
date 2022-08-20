package node_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
)

func TestNodeLifecycle(t *testing.T) {
	acctesting.SkipPlatformCI(t, "darwin", "requires setting up localhost aliases")

	// Configure
	partitions, daemons := acctesting.CreateTestNet(t, 1, 1, 0, false)

	// Start
	for _, netName := range partitions {
		for _, daemon := range daemons[netName] {
			require.NoError(t, daemon.Start())
		}
	}

	// Stop
	for _, netName := range partitions {
		for _, daemon := range daemons[netName] {
			assert.NoError(t, daemon.Stop())
		}
	}

	// procfs is a linux thing
	if runtime.GOOS != "linux" {
		return
	}

	fds := filepath.Join("/proc", fmt.Sprint(os.Getpid()), "fd")
	entries, err := os.ReadDir(fds)
	require.NoError(t, err)
	for _, e := range entries {
		if e.Type()&os.ModeSymlink == 0 {
			continue
		}

		file, err := filepath.EvalSymlinks(filepath.Join(fds, e.Name()))
		if err != nil {
			continue
		}

		rel, err := filepath.Rel(daemons[partitions[0]][0].Config.RootDir, file)
		require.NoError(t, err)

		if strings.HasPrefix(rel, "../") {
			continue
		}

		assert.Failf(t, "Files are still open after the node was shut down", "%q is open", rel)
	}
}
