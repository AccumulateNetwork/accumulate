package server

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Path validation helpers to prevent directory traversal attacks

// ValidatePath ensures a path is safe and within allowed boundaries
func ValidatePath(path string, allowedBase string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	// Clean the path to resolve any . or .. components
	cleanPath := filepath.Clean(path)

	// Convert to absolute path
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	// If allowedBase is provided, ensure path is within it
	if allowedBase != "" {
		absBase, err := filepath.Abs(filepath.Clean(allowedBase))
		if err != nil {
			return "", fmt.Errorf("failed to resolve base path: %w", err)
		}

		// Check that the resolved path starts with the allowed base
		if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
			return "", fmt.Errorf("path escapes allowed directory: %s is not within %s", absPath, absBase)
		}
	}

	return absPath, nil
}

// ValidatePathExists validates a path and checks it exists
func ValidatePathExists(path string, allowedBase string) (string, error) {
	absPath, err := ValidatePath(path, allowedBase)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return "", fmt.Errorf("path does not exist: %s", absPath)
	}

	return absPath, nil
}

// ValidateDirectoryPath validates a path is a directory
func ValidateDirectoryPath(path string, allowedBase string) (string, error) {
	absPath, err := ValidatePathExists(path, allowedBase)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat path: %w", err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", absPath)
	}

	return absPath, nil
}

// ValidateFilePath validates a path is a file
func ValidateFilePath(path string, allowedBase string) (string, error) {
	absPath, err := ValidatePathExists(path, allowedBase)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat path: %w", err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, expected file: %s", absPath)
	}

	return absPath, nil
}

// ValidateNewFilePath validates a path for a new file (parent must exist)
func ValidateNewFilePath(path string, allowedBase string) (string, error) {
	absPath, err := ValidatePath(path, allowedBase)
	if err != nil {
		return "", err
	}

	// Check parent directory exists
	parentDir := filepath.Dir(absPath)
	if _, err := os.Stat(parentDir); os.IsNotExist(err) {
		return "", fmt.Errorf("parent directory does not exist: %s", parentDir)
	}

	return absPath, nil
}

// String validation helpers

// validNetworkPattern matches valid network names
var validNetworkPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// ValidateNetworkName validates a network name
func ValidateNetworkName(network string) error {
	if network == "" {
		return fmt.Errorf("network name cannot be empty")
	}
	if len(network) > 64 {
		return fmt.Errorf("network name too long (max 64 characters)")
	}
	if !validNetworkPattern.MatchString(network) {
		return fmt.Errorf("invalid network name: must start with letter, contain only alphanumeric, hyphens, underscores")
	}
	return nil
}

// validContainerNamePattern matches valid Docker container names
var validContainerNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// ValidateContainerName validates a Docker container name
func ValidateContainerName(name string) error {
	if name == "" {
		return fmt.Errorf("container name cannot be empty")
	}
	if len(name) > 128 {
		return fmt.Errorf("container name too long (max 128 characters)")
	}
	if !validContainerNamePattern.MatchString(name) {
		return fmt.Errorf("invalid container name: must start with alphanumeric, contain only alphanumeric, dots, hyphens, underscores")
	}
	return nil
}

// validMultiaddrPattern matches valid libp2p multiaddr format
// Example: /dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooW...
var validMultiaddrPattern = regexp.MustCompile(`^(/[a-zA-Z0-9]+/[a-zA-Z0-9._-]+)+$`)

// ValidateMultiaddr validates a libp2p multiaddr
func ValidateMultiaddr(addr string) error {
	if addr == "" {
		return fmt.Errorf("multiaddr cannot be empty")
	}
	if len(addr) > 512 {
		return fmt.Errorf("multiaddr too long (max 512 characters)")
	}
	if !validMultiaddrPattern.MatchString(addr) {
		return fmt.Errorf("invalid multiaddr format")
	}
	return nil
}

// validDockerImagePattern matches valid Docker image names
// Examples: nginx, nginx:latest, registry.gitlab.com/org/repo:v1.0, localhost:5000/image
var validDockerImagePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._\-/:]*[a-zA-Z0-9]$`)

// ValidateDockerImage validates a Docker image name
func ValidateDockerImage(image string) error {
	if image == "" {
		return fmt.Errorf("docker image cannot be empty")
	}
	if len(image) > 256 {
		return fmt.Errorf("docker image name too long (max 256 characters)")
	}
	if !validDockerImagePattern.MatchString(image) {
		return fmt.Errorf("invalid docker image name: must contain only alphanumeric, dots, hyphens, colons, slashes")
	}
	// Check for path traversal attempts
	if strings.Contains(image, "..") {
		return fmt.Errorf("invalid docker image name: contains '..'")
	}
	return nil
}

// ShellEscape escapes a string for safe use in shell scripts
func ShellEscape(s string) string {
	// Use single quotes and escape any single quotes within
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// ValidateAndEscapeForShell validates and escapes a value for shell use
func ValidateAndEscapeForShell(s string, maxLen int) (string, error) {
	if len(s) > maxLen {
		return "", fmt.Errorf("value too long (max %d characters)", maxLen)
	}
	// Check for null bytes which could cause issues
	if strings.Contains(s, "\x00") {
		return "", fmt.Errorf("value contains null bytes")
	}
	return ShellEscape(s), nil
}
