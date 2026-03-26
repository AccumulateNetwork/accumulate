package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		allowedBase string
		wantErr     bool
		errContains string
	}{
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
			errContains: "cannot be empty",
		},
		{
			name:    "valid absolute path",
			path:    "/tmp/test",
			wantErr: false,
		},
		{
			name:    "valid relative path",
			path:    "test/path",
			wantErr: false,
		},
		{
			name:        "path within allowed base",
			path:        "/tmp/allowed/subdir",
			allowedBase: "/tmp/allowed",
			wantErr:     false,
		},
		{
			name:        "path escapes allowed base",
			path:        "/tmp/allowed/../escape",
			allowedBase: "/tmp/allowed",
			wantErr:     true,
			errContains: "escapes allowed directory",
		},
		{
			name:        "path with double dots escaping",
			path:        "/tmp/allowed/../../etc/passwd",
			allowedBase: "/tmp/allowed",
			wantErr:     true,
			errContains: "escapes allowed directory",
		},
		{
			name:        "path equals allowed base",
			path:        "/tmp/allowed",
			allowedBase: "/tmp/allowed",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidatePath(tt.path, tt.allowedBase)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidatePath() expected error, got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidatePath() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidatePath() unexpected error: %v", err)
				}
				if result == "" {
					t.Errorf("ValidatePath() returned empty path")
				}
			}
		})
	}
}

func TestValidatePathExists(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "validation_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file
	testFile := filepath.Join(tempDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name        string
		path        string
		wantErr     bool
		errContains string
	}{
		{
			name:    "existing directory",
			path:    tempDir,
			wantErr: false,
		},
		{
			name:    "existing file",
			path:    testFile,
			wantErr: false,
		},
		{
			name:        "non-existent path",
			path:        filepath.Join(tempDir, "nonexistent"),
			wantErr:     true,
			errContains: "does not exist",
		},
		{
			name:        "empty path",
			path:        "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidatePathExists(tt.path, "")
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidatePathExists() expected error, got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidatePathExists() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidatePathExists() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateDirectoryPath(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "validation_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file
	testFile := filepath.Join(tempDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name        string
		path        string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid directory",
			path:    tempDir,
			wantErr: false,
		},
		{
			name:        "file instead of directory",
			path:        testFile,
			wantErr:     true,
			errContains: "not a directory",
		},
		{
			name:        "non-existent path",
			path:        filepath.Join(tempDir, "nonexistent"),
			wantErr:     true,
			errContains: "does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateDirectoryPath(tt.path, "")
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateDirectoryPath() expected error, got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateDirectoryPath() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateDirectoryPath() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateFilePath(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "validation_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file
	testFile := filepath.Join(tempDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name        string
		path        string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid file",
			path:    testFile,
			wantErr: false,
		},
		{
			name:        "directory instead of file",
			path:        tempDir,
			wantErr:     true,
			errContains: "is a directory",
		},
		{
			name:        "non-existent file",
			path:        filepath.Join(tempDir, "nonexistent.txt"),
			wantErr:     true,
			errContains: "does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateFilePath(tt.path, "")
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateFilePath() expected error, got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateFilePath() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateFilePath() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateNewFilePath(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "validation_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name        string
		path        string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid new file path",
			path:    filepath.Join(tempDir, "newfile.txt"),
			wantErr: false,
		},
		{
			name:        "parent directory does not exist",
			path:        filepath.Join(tempDir, "nonexistent", "newfile.txt"),
			wantErr:     true,
			errContains: "parent directory does not exist",
		},
		{
			name:        "empty path",
			path:        "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateNewFilePath(tt.path, "")
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateNewFilePath() expected error, got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateNewFilePath() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateNewFilePath() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateNetworkName(t *testing.T) {
	tests := []struct {
		name        string
		network     string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid network name - mainnet",
			network: "mainnet",
			wantErr: false,
		},
		{
			name:    "valid network name - testnet",
			network: "testnet",
			wantErr: false,
		},
		{
			name:    "valid network name with underscore",
			network: "main_net",
			wantErr: false,
		},
		{
			name:    "valid network name with hyphen",
			network: "main-net",
			wantErr: false,
		},
		{
			name:    "valid network name with numbers",
			network: "testnet2",
			wantErr: false,
		},
		{
			name:        "empty network name",
			network:     "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "network name starting with number",
			network:     "1testnet",
			wantErr:     true,
			errContains: "must start with letter",
		},
		{
			name:        "network name with spaces",
			network:     "main net",
			wantErr:     true,
			errContains: "must start with letter",
		},
		{
			name:        "network name with special characters",
			network:     "main@net",
			wantErr:     true,
			errContains: "must start with letter",
		},
		{
			name:        "network name too long",
			network:     strings.Repeat("a", 65),
			wantErr:     true,
			errContains: "too long",
		},
		{
			name:    "network name at max length",
			network: strings.Repeat("a", 64),
			wantErr: false,
		},
		{
			name:        "network name with command injection attempt",
			network:     "mainnet; rm -rf /",
			wantErr:     true,
			errContains: "must start with letter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNetworkName(tt.network)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateNetworkName() expected error, got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateNetworkName() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateNetworkName() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateContainerName(t *testing.T) {
	tests := []struct {
		name        string
		container   string
		wantErr     bool
		errContains string
	}{
		{
			name:      "valid container name",
			container: "accumulate-follower",
			wantErr:   false,
		},
		{
			name:      "valid container name with dots",
			container: "my.container.name",
			wantErr:   false,
		},
		{
			name:      "valid container name with underscores",
			container: "my_container_name",
			wantErr:   false,
		},
		{
			name:      "valid container name starting with number",
			container: "123container",
			wantErr:   false,
		},
		{
			name:        "empty container name",
			container:   "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "container name starting with hyphen",
			container:   "-container",
			wantErr:     true,
			errContains: "must start with alphanumeric",
		},
		{
			name:        "container name starting with dot",
			container:   ".container",
			wantErr:     true,
			errContains: "must start with alphanumeric",
		},
		{
			name:        "container name with spaces",
			container:   "my container",
			wantErr:     true,
			errContains: "must start with alphanumeric",
		},
		{
			name:        "container name with special characters",
			container:   "container@name",
			wantErr:     true,
			errContains: "must start with alphanumeric",
		},
		{
			name:        "container name too long",
			container:   strings.Repeat("a", 129),
			wantErr:     true,
			errContains: "too long",
		},
		{
			name:      "container name at max length",
			container: strings.Repeat("a", 128),
			wantErr:   false,
		},
		{
			name:        "container name with command injection attempt",
			container:   "container; rm -rf /",
			wantErr:     true,
			errContains: "must start with alphanumeric",
		},
		{
			name:        "container name with backticks",
			container:   "container`id`",
			wantErr:     true,
			errContains: "must start with alphanumeric",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContainerName(tt.container)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateContainerName() expected error, got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateContainerName() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateContainerName() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateMultiaddr(t *testing.T) {
	tests := []struct {
		name        string
		addr        string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid DNS multiaddr",
			addr:    "/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx",
			wantErr: false,
		},
		{
			name:    "valid IP4 multiaddr",
			addr:    "/ip4/192.168.1.1/tcp/16593/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx",
			wantErr: false,
		},
		{
			name:    "valid short multiaddr",
			addr:    "/ip4/127.0.0.1/tcp/8080",
			wantErr: false,
		},
		{
			name:        "empty multiaddr",
			addr:        "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "multiaddr without leading slash",
			addr:        "ip4/127.0.0.1/tcp/8080",
			wantErr:     true,
			errContains: "invalid multiaddr format",
		},
		{
			name:        "multiaddr too long",
			addr:        "/dns/" + strings.Repeat("a", 510),
			wantErr:     true,
			errContains: "too long",
		},
		{
			name:    "multiaddr at max length",
			addr:    "/dns/" + strings.Repeat("a", 500) + "/tcp/80",
			wantErr: false,
		},
		{
			name:        "multiaddr with special characters",
			addr:        "/dns/example.com; rm -rf //tcp/80",
			wantErr:     true,
			errContains: "invalid multiaddr format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMultiaddr(tt.addr)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateMultiaddr() expected error, got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateMultiaddr() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateMultiaddr() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateDockerImage(t *testing.T) {
	tests := []struct {
		name        string
		image       string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid simple image",
			image:   "nginx",
			wantErr: false,
		},
		{
			name:    "valid image with tag",
			image:   "nginx:latest",
			wantErr: false,
		},
		{
			name:    "valid image with version tag",
			image:   "nginx:1.21.0",
			wantErr: false,
		},
		{
			name:    "valid registry image",
			image:   "registry.gitlab.com/accumulatenetwork/accumulate:v1.4.0",
			wantErr: false,
		},
		{
			name:    "valid localhost registry",
			image:   "localhost:5000/myimage:latest",
			wantErr: false,
		},
		{
			name:    "valid multi-level path",
			image:   "gcr.io/project/subdir/image:tag",
			wantErr: false,
		},
		{
			name:        "empty image",
			image:       "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "image starting with hyphen",
			image:       "-nginx",
			wantErr:     true,
			errContains: "must contain only",
		},
		{
			name:        "image ending with hyphen",
			image:       "nginx-",
			wantErr:     true,
			errContains: "must contain only",
		},
		{
			name:        "image with path traversal",
			image:       "nginx/../../../etc/passwd",
			wantErr:     true,
			errContains: "contains '..'",
		},
		{
			name:        "image too long",
			image:       "a" + strings.Repeat("b", 256),
			wantErr:     true,
			errContains: "too long",
		},
		{
			name:    "image at max length",
			image:   "a" + strings.Repeat("b", 254) + "c",
			wantErr: false,
		},
		{
			name:        "image with spaces",
			image:       "nginx latest",
			wantErr:     true,
			errContains: "must contain only",
		},
		{
			name:        "image with command injection",
			image:       "nginx; rm -rf /",
			wantErr:     true,
			errContains: "must contain only",
		},
		{
			name:        "single character image",
			image:       "a",
			wantErr:     true,
			errContains: "must contain only",
		},
		{
			name:    "two character image",
			image:   "ab",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDockerImage(tt.image)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateDockerImage() expected error, got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateDockerImage() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateDockerImage() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestShellEscape(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple string",
			input:    "hello",
			expected: "'hello'",
		},
		{
			name:     "string with spaces",
			input:    "hello world",
			expected: "'hello world'",
		},
		{
			name:     "string with single quote",
			input:    "it's",
			expected: "'it'\"'\"'s'",
		},
		{
			name:     "string with multiple single quotes",
			input:    "it's a 'test'",
			expected: "'it'\"'\"'s a '\"'\"'test'\"'\"''",
		},
		{
			name:     "string with special characters",
			input:    "$HOME; rm -rf /",
			expected: "'$HOME; rm -rf /'",
		},
		{
			name:     "string with backticks",
			input:    "`id`",
			expected: "'`id`'",
		},
		{
			name:     "string with newline",
			input:    "line1\nline2",
			expected: "'line1\nline2'",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "''",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShellEscape(tt.input)
			if result != tt.expected {
				t.Errorf("ShellEscape() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestValidateAndEscapeForShell(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		maxLen      int
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid string within limit",
			input:   "hello",
			maxLen:  10,
			wantErr: false,
		},
		{
			name:    "string at exact limit",
			input:   "hello",
			maxLen:  5,
			wantErr: false,
		},
		{
			name:        "string exceeds limit",
			input:       "hello world",
			maxLen:      5,
			wantErr:     true,
			errContains: "too long",
		},
		{
			name:        "string with null byte",
			input:       "hello\x00world",
			maxLen:      100,
			wantErr:     true,
			errContains: "null bytes",
		},
		{
			name:    "empty string",
			input:   "",
			maxLen:  10,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateAndEscapeForShell(tt.input, tt.maxLen)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateAndEscapeForShell() expected error, got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateAndEscapeForShell() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateAndEscapeForShell() unexpected error: %v", err)
				}
				// Verify result is properly escaped
				if result == "" && tt.input != "" {
					t.Errorf("ValidateAndEscapeForShell() returned empty result for non-empty input")
				}
			}
		})
	}
}

// Test directory traversal attack prevention
func TestDirectoryTraversalPrevention(t *testing.T) {
	// Create a temporary directory structure
	tempDir, err := os.MkdirTemp("", "traversal_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	allowedDir := filepath.Join(tempDir, "allowed")
	if err := os.MkdirAll(allowedDir, 0755); err != nil {
		t.Fatalf("Failed to create allowed dir: %v", err)
	}

	// Create a file outside allowed directory
	outsideFile := filepath.Join(tempDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("Failed to create outside file: %v", err)
	}

	traversalAttempts := []string{
		filepath.Join(allowedDir, "..", "outside.txt"),
		filepath.Join(allowedDir, "..", "..", "etc", "passwd"),
		allowedDir + "/../../outside.txt",
		allowedDir + "/../outside.txt",
	}

	for _, attempt := range traversalAttempts {
		t.Run("attempt: "+attempt, func(t *testing.T) {
			_, err := ValidatePath(attempt, allowedDir)
			if err == nil {
				t.Errorf("ValidatePath() should have rejected path traversal attempt: %s", attempt)
			}
			if !strings.Contains(err.Error(), "escapes") {
				t.Errorf("ValidatePath() error should mention 'escapes', got: %v", err)
			}
		})
	}
}
