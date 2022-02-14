package main

import (
	_ "embed"
)

//the c template will include source + header with a ACME_HEADER guard between source and header.
//go:embed c.tmpl
var cHeader string
var _ = Templates.Register(cHeader, "c", nil, "c-header")

//enum_gen.c
//#ifdef ACME_HEADER
//#undef ACME_HEADER
//#endif
//#define ACME_HEADER
//#include "enum_gen.h"
//#undef ACME_HEADER
