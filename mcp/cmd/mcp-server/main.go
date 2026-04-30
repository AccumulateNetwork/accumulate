// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"log"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/mcp/server"
)

func main() {
	var (
		logFile = flag.String("log", "", "Log file path (default: stderr)")
		http    = flag.Bool("http", false, "Run as HTTP server instead of stdio")
		host    = flag.String("host", "127.0.0.1", "HTTP server host (only with -http)")
		port    = flag.String("port", "8080", "HTTP server port (only with -http)")
	)
	flag.Parse()

	// Set up logging
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalf("failed to open log file: %v", err)
		}
		defer f.Close()
		log.SetOutput(f)
	}

	// Create MCP server
	srv := server.NewServer()

	if *http {
		// Run as HTTP server
		instance, err := server.StartHTTPServer(srv, *host, *port)
		if err != nil {
			log.Fatalf("failed to start HTTP server: %v", err)
		}
		defer instance.Close()

		// Wait forever
		select {}
	}

	// Run as stdio server (standard MCP transport)
	runStdioServer(srv)
}

// runStdioServer runs the MCP server over stdin/stdout
func runStdioServer(srv *server.Server) {
	scanner := bufio.NewScanner(os.Stdin)
	// Set a larger buffer for large requests
	const maxSize = 10 * 1024 * 1024 // 10MB
	buf := make([]byte, maxSize)
	scanner.Buffer(buf, maxSize)

	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var request map[string]interface{}
		if err := json.Unmarshal(line, &request); err != nil {
			log.Printf("Error parsing request: %v", err)
			// Send JSON-RPC error response
			response := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      nil,
				"error": map[string]interface{}{
					"code":    -32700,
					"message": "Parse error",
				},
			}
			encoder.Encode(response)
			continue
		}

		response := srv.HandleRequest(request)
		if err := encoder.Encode(response); err != nil {
			log.Printf("Error encoding response: %v", err)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Scanner error: %v", err)
	}
}
