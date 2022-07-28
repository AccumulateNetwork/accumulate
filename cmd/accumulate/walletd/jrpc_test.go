package walletd

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// listenHttpUrl takes a string such as `http://localhost:123` and creates a TCP
// listener.
func listenHttpUrl(s string) (net.Listener, bool, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, false, fmt.Errorf("invalid address: %v", err)
	}

	if u.Path != "" && u.Path != "/" {
		return nil, false, fmt.Errorf("invalid address: path is not empty")
	}

	var secure bool
	switch u.Scheme {
	case "tcp", "http":
		secure = false
	case "https":
		secure = true
	default:
		return nil, false, fmt.Errorf("invalid address: unsupported scheme %q", u.Scheme)
	}

	l, err := net.Listen("tcp", u.Host)
	if err != nil {
		return nil, false, err
	}

	return l, secure, nil
}

var walletEndpoint = "http://0.0.0.0:33322"

func LaunchWalletServer() (*JrpcMethods, error) {
	// Create the JSON-RPC handler
	jrpc, err := NewJrpc(Options{
		TxMaxWaitTime: time.Minute,
	})

	require.NoError(t, err)

	// Run JSON-RPC server
	api := &http.Server{Handler: jrpc.NewMux()}
	l, secure, err := listenHttpUrl(walletEndpoint)
	if err != nil {
		return nil, err
	}
	if secure {
		return nil, fmt.Errorf("currently doesn't support secure server")
	}

	go func() {
		err := api.Serve(l)
		if err != nil {
			jrpc.Logger.Error("JSON-RPC server", "err", err)
		}
	}()
	return nil
}
func TestWallet(t *testing.T) {
	// Create the JSON-RPC handler
	jrpc, err := NewJrpc(Options{
		TxMaxWaitTime: time.Minute,
	})

	require.NoError(t, err)
	//mux := j.NewMux()

	// Run JSON-RPC server
	api := &http.Server{Handler: jrpc.NewMux()}
	l, secure, err := listenHttpUrl("http://0.0.0.0:33322")
	require.NoError(t, err)
	require.False(t, secure)

	go func() {
		err := api.Serve(l)
		if err != nil {
			jrpc.Logger.Error("JSON-RPC server", "err", err)
		}
	}()
}
