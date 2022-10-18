package walletd

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWallet(t *testing.T) {
	// Create the JSON-RPC handler
	jrpc, err := NewJrpc(Options{
		TxMaxWaitTime: time.Minute,
	})

	require.NoError(t, err)

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

	t.Cleanup(func() { _ = api.Shutdown(context.Background()) })
}
