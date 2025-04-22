// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// +build ignore

// This file is a test runner for address2.go only
package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	// Create a temporary directory for our test
	cmd := exec.Command("mkdir", "-p", "/tmp/address2_test")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Println("Error creating temp directory:", err)
		os.Exit(1)
	}

	// Copy address2.go to the temp directory
	cmd = exec.Command("cp", "address2.go", "/tmp/address2_test/address2.go")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Println("Error copying address2.go:", err)
		os.Exit(1)
	}

	// Copy address2_only_test.go to the temp directory
	cmd = exec.Command("cp", "address2_only_test.go", "/tmp/address2_test/address2_test.go")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Println("Error copying address2_only_test.go:", err)
		os.Exit(1)
	}

	// Run the tests in the temp directory
	cmd = exec.Command("go", "test", "-v", "/tmp/address2_test")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = "/tmp/address2_test"
	if err := cmd.Run(); err != nil {
		fmt.Println("Tests failed:", err)
		os.Exit(1)
	}

	fmt.Println("All tests passed!")
}
