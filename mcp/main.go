package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gitlab.com/accumulatenetwork/accumulate/mcp/server"
)

func main() {
	// Command-line flags
	httpPort := flag.String("http-port", "", "HTTP server port (default: stdio mode)")
	httpHost := flag.String("http-host", "localhost", "HTTP server host (default: localhost)")
	flag.Parse()

	// Create MCP server
	srv := server.NewServer()

	log.SetOutput(os.Stderr)

	// Check if HTTP mode is enabled
	if *httpPort != "" {
		// Start HTTP server
		httpServer, err := server.StartHTTPServer(srv, *httpHost, *httpPort)
		if err != nil {
			log.Fatalf("Failed to start HTTP server: %v", err)
		}

		log.Printf("Accumulate MCP server running in HTTP mode on http://%s", httpServer.Addr)

		// Wait for interrupt signal
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting down...")
		if err := httpServer.Close(); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
		return
	}

	// Default: stdio mode
	log.Println("Accumulate MCP server starting on stdio")

	// Set up JSON-RPC communication over stdio
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for {
		var request map[string]interface{}
		if err := decoder.Decode(&request); err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("Error decoding request: %v", err)
			continue
		}

		response := srv.HandleRequest(request)

		if err := encoder.Encode(response); err != nil {
			log.Printf("Error encoding response: %v", err)
			continue
		}
	}

	fmt.Fprintln(os.Stderr, "Accumulate MCP server shutting down")
}
