package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// validGitRefPattern validates git refs to prevent command injection
// Allows: alphanumeric, dots, hyphens, underscores, forward slashes
// Examples: main, v1.4.0, feature/my-branch, HEAD~1
var validGitRefPattern = regexp.MustCompile(`^[a-zA-Z0-9._\-/~^]+$`)

// validateGitRef checks if a git ref is safe to use in commands
func validateGitRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("git ref cannot be empty")
	}
	if len(ref) > 256 {
		return fmt.Errorf("git ref too long (max 256 characters)")
	}
	if !validGitRefPattern.MatchString(ref) {
		return fmt.Errorf("invalid git ref: contains disallowed characters (allowed: alphanumeric, . - _ / ~ ^)")
	}
	// Prevent path traversal
	if strings.Contains(ref, "..") {
		return fmt.Errorf("invalid git ref: contains '..'")
	}
	return nil
}

// buildBinary builds the accumulated binary from source
func (s *Server) buildBinary(args map[string]interface{}) (map[string]interface{}, error) {
	// Get repository path (default to ../.. from MCP directory)
	repoPath, ok := args["repo_path"].(string)
	if !ok || repoPath == "" {
		// Default to accumulate repository root (two levels up from mcp/)
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
		repoPath = filepath.Join(cwd, "..", "..")
	}

	// Resolve to absolute path
	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve repository path: %w", err)
	}

	// Verify it's a git repository
	gitDir := filepath.Join(absRepoPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("not a git repository: %s", absRepoPath)
	}

	// Get ref (branch, tag, or commit)
	ref, _ := args["ref"].(string)
	if ref == "" {
		// Use current checkout - get current branch/commit
		cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
		cmd.Dir = absRepoPath
		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("failed to get current branch: %w", err)
		}
		ref = strings.TrimSpace(string(output))
	}

	// Validate the ref to prevent command injection
	if err := validateGitRef(ref); err != nil {
		return nil, fmt.Errorf("invalid ref parameter: %w", err)
	}

	// Check if we need to checkout a different ref
	needsCheckout := false
	if refArg, ok := args["ref"].(string); ok && refArg != "" {
		needsCheckout = true
	}

	// Store current ref to restore later if we checked out
	var originalRef string
	if needsCheckout {
		cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
		cmd.Dir = absRepoPath
		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("failed to get current ref: %w", err)
		}
		originalRef = strings.TrimSpace(string(output))

		// Checkout the requested ref (already validated above)
		cmd = exec.Command("git", "checkout", ref)
		cmd.Dir = absRepoPath
		output, err = cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("failed to checkout %s: %w\nOutput: %s", ref, err, string(output))
		}
	}

	// Defer restoring original ref if we changed it
	if needsCheckout && originalRef != "" {
		defer func() {
			cmd := exec.Command("git", "checkout", originalRef)
			cmd.Dir = absRepoPath
			_ = cmd.Run() // Best effort restore
		}()
	}

	// Get output path
	outputPath, _ := args["output_path"].(string)
	if outputPath == "" {
		// Default to /tmp/accumulated-<ref>
		safeName := strings.ReplaceAll(ref, "/", "-")
		outputPath = filepath.Join("/tmp", "accumulated-"+safeName)
	}

	// Resolve output path
	absOutputPath, err := filepath.Abs(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve output path: %w", err)
	}

	// Check if clean is requested
	clean, _ := args["clean"].(bool)
	if clean {
		cmd := exec.Command("go", "clean")
		cmd.Dir = filepath.Join(absRepoPath, "cmd", "accumulated")
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("failed to run go clean: %w", err)
		}
	}

	// Build the binary with timeout (10 minutes max)
	// Change to cmd/accumulated directory and build
	buildDir := filepath.Join(absRepoPath, "cmd", "accumulated")
	buildTimeout := 10 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", absOutputPath, ".")
	cmd.Dir = buildDir
	cmd.Env = os.Environ() // Inherit environment

	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("build timed out after %v", buildTimeout)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to build binary: %w\nOutput: %s", err, string(output))
	}

	// Verify the binary was created
	if _, err := os.Stat(absOutputPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("binary was not created at %s", absOutputPath)
	}

	// Get binary info
	info, err := os.Stat(absOutputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat binary: %w", err)
	}

	// Get version from binary
	versionCmd := exec.Command(absOutputPath, "version")
	versionOutput, err := versionCmd.CombinedOutput()
	version := "unknown"
	if err == nil {
		version = strings.TrimSpace(string(versionOutput))
	}

	// Get commit hash
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = absRepoPath
	commitOutput, err := cmd.CombinedOutput()
	commit := "unknown"
	if err == nil {
		commit = strings.TrimSpace(string(commitOutput))[:8] // Short hash
	}

	return map[string]interface{}{
		"success":     true,
		"binary_path": absOutputPath,
		"ref":         ref,
		"commit":      commit,
		"size_bytes":  info.Size(),
		"version":     version,
		"build_output": string(output),
	}, nil
}
