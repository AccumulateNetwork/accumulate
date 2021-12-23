package cmd

import (
	"encoding/json"
	"testing"
)

func keysTest(t *testing.T, tc *testCmd) {

	keyGenCommandList := []string{"key generate testkey1",
		"key generate testkey2",
		"key generate testkey3",
		"key generate testkey4",
		"key generate testkey5"}

	for _, v := range keyGenCommandList {
		out, err := tc.execute(t, v)
		if err != nil {
			t.Fatal(err)
		}

		var result map[string]interface{}

		// Unmarshal or Decode the JSON to the interface.
		json.Unmarshal([]byte(out), &result)

		if string(out) != "hi-via-args" {
			t.Fatalf("expected \"%s\" got \"%s\"", "hi-via-args", string(out))
		}
	}

	argsKeyList := "key list"
	out, err := tc.execute(t, argsKeyList)
	if err != nil {
		t.Fatal(err)
	}

	if string(out) != "hi-via-args" {
		t.Fatalf("expected \"%s\" got \"%s\"", "hi-via-args", string(out))
	}
}
