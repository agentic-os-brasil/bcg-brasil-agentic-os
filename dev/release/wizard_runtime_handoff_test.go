package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWizardKeepsTheCanarySimplePathLinear(t *testing.T) {
	root := filepath.Join("..", "..", "installers", "wizard")
	index, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := os.ReadFile(filepath.Join(root, "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`data-choice="new"`,
		`data-choice="update"`,
		`data-screen="progress"`,
		`data-screen="complete"`,
		`data-action="open-claude"`,
		"Abrir Claude Desktop",
		"Tentar novamente",
	} {
		if !strings.Contains(string(index), required) {
			t.Fatalf("wizard simple path is missing %q", required)
		}
	}
	for _, required := range []string{
		"/api/simple-install",
		"/api/verify",
		"/api/install",
		"/api/create-workspace",
		"/api/launch-runtime",
		"terminalFailure",
	} {
		if !strings.Contains(string(app), required) {
			t.Fatalf("wizard simple behavior is missing %q", required)
		}
	}
}

func TestWizardHidesTechnicalReleaseDetailsFromTheUserPath(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "installers", "wizard", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, forbidden := range []string{
		"local_beta",
		"Authenticode",
		"canary qualification",
		"issuer",
		"release authorization",
		"runtime-ready-modal",
	} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Fatalf("wizard user path exposes technical detail %q", forbidden)
		}
	}
}
