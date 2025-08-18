//go:build ignore
// +build ignore

// This file demonstrates the "package 2 is not in std" error
// DO NOT FIX THIS FILE - It's intentionally broken for demonstration purposes

package main

import (
	"fmt"
)

func main() {
	// Call a function that doesn't exist
	result := nonExistentFunction()
	fmt.Println(result)
	
	// Use an undefined variable
	fmt.Println(undefinedVariable)
	
	// Call another missing function
	anotherMissingFunction(42)
}

// Missing function definitions:
// func nonExistentFunction() string
// func anotherMissingFunction(int)
// var undefinedVariable string