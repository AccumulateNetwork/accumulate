package main

import (
	_ "embed"
)

//go:embed go.tmpl
var goSrc string

var Go = mustParseTemplate("go.tmpl", goSrc, nil)
