package main

import (
	"encoding/json"
	"io"
	"os"
	"testing"
)

func TestClientCapabilitiesReadOnlyWithoutInstallation(t *testing.T) {
	oldArgs, oldVersion, oldPin, oldOutput := os.Args, Version, AuthorityRegistrySHA256, os.Stdout
	defer func() { os.Args, Version, AuthorityRegistrySHA256, os.Stdout = oldArgs, oldVersion, oldPin, oldOutput }()
	Version, AuthorityRegistrySHA256 = "0.3.7", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	os.Args = []string{"maestro-install", "client-capabilities"}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	os.Stdout = w
	err = run()
	w.Close()
	os.Stdout = oldOutput
	body, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err != nil {
		t.Fatalf("read-only capability discovery requires no installation: %v", err)
	}
	var value struct {
		Schema         int    `json:"schema_version"`
		Profile        string `json:"profile"`
		Protocol       string `json:"protocol"`
		Version        string `json:"version"`
		CarrierVersion string `json:"carrier_version"`
	}
	if err = json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	if value.Schema != 1 || value.Profile != "stable-local-v1" || value.Protocol != "maestro-stable-carrier-v1" || value.Version != "0.3.7" || value.CarrierVersion != "1.0.0" {
		t.Fatalf("unexpected actual capability: %s", body)
	}
}
