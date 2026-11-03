package main

import (
	"encoding/json"
	"io"
	"os"
	"testing"
)

func TestServerClientCapabilitiesWithoutInstallation(t *testing.T) {
	oldArgs, oldOut := os.Args, os.Stdout
	defer func() { os.Args, os.Stdout = oldArgs, oldOut }()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	os.Args = []string{"maestro-mcp", "client-capabilities"}
	os.Stdout = w
	err = run()
	w.Close()
	os.Stdout = oldOut
	body, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err != nil {
		t.Fatalf("public capabilities require no installed binding: %v", err)
	}
	var c map[string]any
	if err = json.Unmarshal(body, &c); err != nil {
		t.Fatal(err)
	}
	if c["component"] != "server" || c["profile"] != "stable-local-v1" || c["protocol"] != "maestro-stable-carrier-v1" || c["carrier_version"] != "1.0.0" {
		t.Fatalf("actual capability differs: %s", body)
	}
}
