package server

import (
	"context"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/mcp/client"
)

// ClientHelper provides shared functionality for tool implementations
type ClientHelper struct {
	state *State
	cfg   *Config
}

// NewClientHelper creates a new client helper
func NewClientHelper(state *State) *ClientHelper {
	return &ClientHelper{
		state: state,
		cfg:   LoadConfig(),
	}
}

// WithClient creates a client, executes the function, and ensures cleanup
func (h *ClientHelper) WithClient(network string, fn func(ctx context.Context, c *client.Client) error) error {
	if network == "" {
		network = h.state.GetNetwork()
	}

	c, err := client.NewClient(network)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), h.cfg.APITimeout)
	defer cancel()

	return fn(ctx, c)
}

// WithClientResult creates a client, executes the function, and returns the result
func (h *ClientHelper) WithClientResult(network string, fn func(ctx context.Context, c *client.Client) (map[string]interface{}, error)) (map[string]interface{}, error) {
	if network == "" {
		network = h.state.GetNetwork()
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), h.cfg.APITimeout)
	defer cancel()

	result, err := fn(ctx, c)
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("operation timed out after %v", h.cfg.APITimeout)
	}
	return result, err
}

// WithClientCustomTimeout creates a client with custom timeout
func (h *ClientHelper) WithClientCustomTimeout(network string, timeout time.Duration, fn func(ctx context.Context, c *client.Client) (map[string]interface{}, error)) (map[string]interface{}, error) {
	if network == "" {
		network = h.state.GetNetwork()
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := fn(ctx, c)
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("operation timed out after %v", timeout)
	}
	return result, err
}

// GetNetwork returns the current network from state or default
func (h *ClientHelper) GetNetwork(args map[string]interface{}) string {
	if network, ok := args["network"].(string); ok && network != "" {
		return network
	}
	return h.state.GetNetwork()
}

// GetStringArg gets a string argument with default value
func GetStringArg(args map[string]interface{}, key string, defaultValue string) string {
	if val, ok := args[key].(string); ok && val != "" {
		return val
	}
	return defaultValue
}

// GetIntArg gets an integer argument with default value
func GetIntArg(args map[string]interface{}, key string, defaultValue int) int {
	if val, ok := args[key].(float64); ok {
		return int(val)
	}
	return defaultValue
}

// GetBoolArg gets a boolean argument with default value
func GetBoolArg(args map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := args[key].(bool); ok {
		return val
	}
	return defaultValue
}

// RequireStringArg gets a required string argument or returns error
func RequireStringArg(args map[string]interface{}, key string) (string, error) {
	val, ok := args[key].(string)
	if !ok || val == "" {
		return "", fmt.Errorf("missing required parameter: %s", key)
	}
	return val, nil
}
