package main

import (
	_ "embed"
)

//go:embed c.tmpl
var cHeader string
var _ = Templates.Register(cHeader, "c-header", nil)

//only doing everything in C header for now, will optimize later
////go:embed c.tmpl
//var cSource string
//var _ = Templates.Register(cHeader, "c-source", nil)
