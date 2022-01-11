package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

var input = flag.String("input", "", "Input YAML file")
var output = flag.String("output", "", "Output JSON file")

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func main() {
	flag.Parse()

	input, err := os.Open(*input)
	check(err)
	defer input.Close()

	output, err := os.Create(*output)
	check(err)
	defer output.Close()

	dec := yaml.NewDecoder(input)
	enc := json.NewEncoder(output)
	enc.SetIndent("", "    ")

	var v interface{}
	check(dec.Decode(&v))
	check(enc.Encode(&v))
}
