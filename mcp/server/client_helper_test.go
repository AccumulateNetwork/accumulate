package server

import (
	"errors"
	"net"
	"testing"
	"time"
)

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxRetries != 3 {
		t.Errorf("DefaultRetryConfig().MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.InitialBackoff != 500*time.Millisecond {
		t.Errorf("DefaultRetryConfig().InitialBackoff = %v, want 500ms", cfg.InitialBackoff)
	}
	if cfg.MaxBackoff != 5*time.Second {
		t.Errorf("DefaultRetryConfig().MaxBackoff = %v, want 5s", cfg.MaxBackoff)
	}
	if cfg.BackoffFactor != 2.0 {
		t.Errorf("DefaultRetryConfig().BackoffFactor = %f, want 2.0", cfg.BackoffFactor)
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "connection refused",
			err:      errors.New("dial tcp: connection refused"),
			expected: true,
		},
		{
			name:     "connection reset",
			err:      errors.New("read: connection reset by peer"),
			expected: true,
		},
		{
			name:     "connection timed out",
			err:      errors.New("dial tcp: connection timed out"),
			expected: true,
		},
		{
			name:     "no such host",
			err:      errors.New("dial tcp: lookup example.com: no such host"),
			expected: true,
		},
		{
			name:     "i/o timeout",
			err:      errors.New("i/o timeout"),
			expected: true,
		},
		{
			name:     "EOF",
			err:      errors.New("unexpected EOF"),
			expected: true,
		},
		{
			name:     "broken pipe",
			err:      errors.New("write: broken pipe"),
			expected: true,
		},
		{
			name:     "network unreachable",
			err:      errors.New("dial tcp: network is unreachable"),
			expected: true,
		},
		{
			name:     "no route to host",
			err:      errors.New("dial tcp: no route to host"),
			expected: true,
		},
		{
			name:     "context deadline exceeded",
			err:      errors.New("context deadline exceeded"),
			expected: true,
		},
		{
			name:     "temporary failure",
			err:      errors.New("temporary failure in name resolution"),
			expected: true,
		},
		{
			name:     "too many open files",
			err:      errors.New("too many open files"),
			expected: true,
		},
		{
			name:     "non-retryable error - invalid argument",
			err:      errors.New("invalid argument"),
			expected: false,
		},
		{
			name:     "non-retryable error - not found",
			err:      errors.New("account not found"),
			expected: false,
		},
		{
			name:     "non-retryable error - permission denied",
			err:      errors.New("permission denied"),
			expected: false,
		},
		{
			name:     "non-retryable error - bad request",
			err:      errors.New("bad request: invalid parameter"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("isRetryableError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// mockNetError implements net.Error interface for testing
type mockNetError struct {
	timeout   bool
	temporary bool
	msg       string
}

func (e *mockNetError) Error() string   { return e.msg }
func (e *mockNetError) Timeout() bool   { return e.timeout }
func (e *mockNetError) Temporary() bool { return e.temporary }

// Ensure mockNetError implements net.Error
var _ net.Error = (*mockNetError)(nil)

func TestIsRetryableError_NetError(t *testing.T) {
	tests := []struct {
		name     string
		err      net.Error
		expected bool
	}{
		{
			name:     "timeout error",
			err:      &mockNetError{timeout: true, temporary: false, msg: "timeout"},
			expected: true,
		},
		{
			name:     "temporary error",
			err:      &mockNetError{timeout: false, temporary: true, msg: "temporary"},
			expected: true,
		},
		{
			name:     "both timeout and temporary",
			err:      &mockNetError{timeout: true, temporary: true, msg: "both"},
			expected: true,
		},
		{
			name:     "neither timeout nor temporary",
			err:      &mockNetError{timeout: false, temporary: false, msg: "permanent"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("isRetryableError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsRetryableError_CaseInsensitive(t *testing.T) {
	// Test that error matching is case-insensitive
	tests := []struct {
		name string
		err  error
	}{
		{name: "uppercase", err: errors.New("CONNECTION REFUSED")},
		{name: "lowercase", err: errors.New("connection refused")},
		{name: "mixed case", err: errors.New("Connection Refused")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !isRetryableError(tt.err) {
				t.Errorf("isRetryableError(%v) should return true regardless of case", tt.err)
			}
		})
	}
}

func TestGetStringArg(t *testing.T) {
	tests := []struct {
		name         string
		args         map[string]interface{}
		key          string
		defaultValue string
		expected     string
	}{
		{
			name:         "key exists with string value",
			args:         map[string]interface{}{"network": "mainnet"},
			key:          "network",
			defaultValue: "testnet",
			expected:     "mainnet",
		},
		{
			name:         "key exists with empty string",
			args:         map[string]interface{}{"network": ""},
			key:          "network",
			defaultValue: "testnet",
			expected:     "testnet",
		},
		{
			name:         "key does not exist",
			args:         map[string]interface{}{},
			key:          "network",
			defaultValue: "testnet",
			expected:     "testnet",
		},
		{
			name:         "key exists with wrong type",
			args:         map[string]interface{}{"network": 123},
			key:          "network",
			defaultValue: "testnet",
			expected:     "testnet",
		},
		{
			name:         "nil args",
			args:         nil,
			key:          "network",
			defaultValue: "testnet",
			expected:     "testnet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetStringArg(tt.args, tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("GetStringArg() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetIntArg(t *testing.T) {
	tests := []struct {
		name         string
		args         map[string]interface{}
		key          string
		defaultValue int
		expected     int
	}{
		{
			name:         "key exists with float64 value",
			args:         map[string]interface{}{"count": float64(42)},
			key:          "count",
			defaultValue: 10,
			expected:     42,
		},
		{
			name:         "key does not exist",
			args:         map[string]interface{}{},
			key:          "count",
			defaultValue: 10,
			expected:     10,
		},
		{
			name:         "key exists with wrong type",
			args:         map[string]interface{}{"count": "42"},
			key:          "count",
			defaultValue: 10,
			expected:     10,
		},
		{
			name:         "key exists with zero",
			args:         map[string]interface{}{"count": float64(0)},
			key:          "count",
			defaultValue: 10,
			expected:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetIntArg(tt.args, tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("GetIntArg() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestGetBoolArg(t *testing.T) {
	tests := []struct {
		name         string
		args         map[string]interface{}
		key          string
		defaultValue bool
		expected     bool
	}{
		{
			name:         "key exists with true",
			args:         map[string]interface{}{"enabled": true},
			key:          "enabled",
			defaultValue: false,
			expected:     true,
		},
		{
			name:         "key exists with false",
			args:         map[string]interface{}{"enabled": false},
			key:          "enabled",
			defaultValue: true,
			expected:     false,
		},
		{
			name:         "key does not exist",
			args:         map[string]interface{}{},
			key:          "enabled",
			defaultValue: true,
			expected:     true,
		},
		{
			name:         "key exists with wrong type",
			args:         map[string]interface{}{"enabled": "true"},
			key:          "enabled",
			defaultValue: false,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetBoolArg(tt.args, tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("GetBoolArg() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRequireStringArg(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]interface{}
		key         string
		expected    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "key exists with value",
			args:     map[string]interface{}{"url": "acc://example"},
			key:      "url",
			expected: "acc://example",
			wantErr:  false,
		},
		{
			name:        "key exists with empty string",
			args:        map[string]interface{}{"url": ""},
			key:         "url",
			wantErr:     true,
			errContains: "missing required parameter",
		},
		{
			name:        "key does not exist",
			args:        map[string]interface{}{},
			key:         "url",
			wantErr:     true,
			errContains: "missing required parameter",
		},
		{
			name:        "key exists with wrong type",
			args:        map[string]interface{}{"url": 123},
			key:         "url",
			wantErr:     true,
			errContains: "missing required parameter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RequireStringArg(tt.args, tt.key)
			if tt.wantErr {
				if err == nil {
					t.Errorf("RequireStringArg() expected error, got nil")
				} else if tt.errContains != "" && !stringContainsSubstr(err.Error(), tt.errContains) {
					t.Errorf("RequireStringArg() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("RequireStringArg() unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("RequireStringArg() = %q, want %q", result, tt.expected)
				}
			}
		})
	}
}

func stringContainsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContainsSubstrHelper(s, substr))
}

func stringContainsSubstrHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
