package main

import (
	"path/filepath"
	"testing"
)

func TestDbRepair(t *testing.T) {
	dir := t.TempDir()
	buildTestDBs(1e5, filepath.Join(dir, "good.db"), filepath.Join(dir, "bad.db"))
	buildSummary(filepath.Join(dir, "good.db"), filepath.Join(dir, "summary"))
}
