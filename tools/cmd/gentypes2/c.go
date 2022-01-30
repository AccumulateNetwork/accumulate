package main

import (
	_ "embed"
)

//go:embed c.tmpl
var cSrc string

var C = mustParseTemplate("c.tmpl", cSrc, nil)
