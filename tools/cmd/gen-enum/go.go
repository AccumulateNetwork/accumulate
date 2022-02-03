package main

import (
	_ "embed"
)

//go:embed go.go.tmpl
var goSrc string

var _ = Templates.Register(goSrc, "go", nil, "Go")
