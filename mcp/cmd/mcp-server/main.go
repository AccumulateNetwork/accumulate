// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"flag"
	"log"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/mcp/server"
)

func main() {
	var (
		endpoint = flag.String("endpoint", server.DefaultEndpoint, "Accumulate API endpoint")
		logFile  = flag.String("log", "", "Log file path (default: no logging)")
	)
	flag.Parse()

	var logger *log.Logger
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalf("failed to open log file: %v", err)
		}
		defer f.Close()
		logger = log.New(f, "[mcp] ", log.LstdFlags)
	}

	srv := server.New(&server.Config{
		Logger: logger,
	})

	srv.RegisterAccumulateTools(*endpoint)

	if err := srv.Run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
