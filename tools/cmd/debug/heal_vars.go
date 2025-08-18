// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"os"
	"path/filepath"
	"time"
)

// Global variables used by heal and sequence commands
var (
	// Flags
	verbose        bool
	only           string
	pretend        bool
	waitForTxn     bool
	healContinuous bool
	pprof          string
	cachedScan     string
	peerDb         string
	debug          bool
	lightDb        string
	
	// Other globals
	homeDir  = getUserHomeDir()
	cacheDir = filepath.Join(homeDir, ".accumulate", "cache")
)

// Variables from heal_anchor.go
var (
	healSince         string
	healSinceDuration time.Duration
	sinceBlockCount   uint64
)

func getUserHomeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return "/tmp"
}