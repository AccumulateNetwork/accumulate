package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildBinary(t *testing.T) {
	// Skip if not in accumulate repository
	repoPath, err := filepath.Abs("../..")
	if err != nil {
		t.Skip("Cannot determine repository path")
	}

	gitDir := filepath.Join(repoPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Skip("Not in accumulate repository")
	}

	s := NewServer()

	// Test 1: Build from current checkout
	t.Run("BuildCurrent", func(t *testing.T) {
		outputPath := filepath.Join(t.TempDir(), "accumulated-test")
		args := map[string]interface{}{
			"output_path": outputPath,
		}

		result, err := s.buildBinary(args)
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}

		// Verify result structure
		if result["success"] != true {
			t.Errorf("Expected success=true, got %v", result["success"])
		}

		binaryPath, ok := result["binary_path"].(string)
		if !ok || binaryPath != outputPath {
			t.Errorf("Expected binary_path=%s, got %v", outputPath, result["binary_path"])
		}

		// Verify binary exists
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			t.Errorf("Binary was not created at %s", outputPath)
		}

		// Verify binary is executable
		info, err := os.Stat(outputPath)
		if err != nil {
			t.Fatalf("Failed to stat binary: %v", err)
		}
		if info.Mode()&0111 == 0 {
			t.Errorf("Binary is not executable")
		}
	})

	// Test 2: Build with explicit repo path
	t.Run("BuildWithRepoPath", func(t *testing.T) {
		outputPath := filepath.Join(t.TempDir(), "accumulated-test2")
		args := map[string]interface{}{
			"repo_path":   repoPath,
			"output_path": outputPath,
		}

		result, err := s.buildBinary(args)
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}

		if result["success"] != true {
			t.Errorf("Expected success=true")
		}

		// Verify binary exists
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			t.Errorf("Binary was not created")
		}
	})

	// Test 3: Invalid repository path
	t.Run("InvalidRepo", func(t *testing.T) {
		args := map[string]interface{}{
			"repo_path": "/nonexistent/path",
		}

		_, err := s.buildBinary(args)
		if err == nil {
			t.Errorf("Expected error for invalid repository, got nil")
		}
	})

	// Test 4: Default output path
	t.Run("DefaultOutputPath", func(t *testing.T) {
		args := map[string]interface{}{}

		result, err := s.buildBinary(args)
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}

		binaryPath, ok := result["binary_path"].(string)
		if !ok {
			t.Fatalf("binary_path not returned")
		}

		// Should be in /tmp
		if !filepath.HasPrefix(binaryPath, "/tmp/accumulated-") {
			t.Errorf("Expected binary in /tmp, got %s", binaryPath)
		}

		// Clean up
		os.Remove(binaryPath)
	})
}
