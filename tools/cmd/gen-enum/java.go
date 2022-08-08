package main

import (
	_ "embed"
)

//go:embed java.tmpl
var javaSrc string

var _ = Templates.Register(javaSrc, "java", nil, "Java")
