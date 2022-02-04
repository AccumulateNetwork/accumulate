package main

import (
	_ "embed"
)

//go:embed c.tmpl
var cSrc string

var C = Templates.Register(cSrc, "c", nil, "c-header")
