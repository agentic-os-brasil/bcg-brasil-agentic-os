package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOfflineFrontendFactoryCLI(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(parent, "frontend")
	var output bytes.Buffer
	if err := run([]string{"--destination", destination, "--version", "0.1.48"}, &output); err != nil {
		t.Fatal(err)
	}
	var report struct{ Name, Mode string }
	if json.Unmarshal(output.Bytes(), &report) != nil || report.Name != "maestro-desktop-methods" || report.Mode != "instructions_only" {
		t.Fatalf("wrong factory report: %s", output.Bytes())
	}
	if _, err := os.Stat(filepath.Join(destination, report.Name, "skills/maestro-onboarding/SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--destination", destination, "--version", "0.1.48"}, &output); err == nil {
		t.Fatal("existing destination replaced")
	}
	for _, args := range [][]string{{}, {"--destination", destination}, {"--destination", destination, "--version", "0.1.48", "extra"}, {"--destination", destination, "--version", "0.1.48", "--namespace", "maestro-product"}, {"--destination", destination, "--version", "0.1.48", "--user-home", parent}} {
		if err := run(args, &output); err == nil {
			t.Fatal("unadmitted factory selection", args)
		}
	}
}
