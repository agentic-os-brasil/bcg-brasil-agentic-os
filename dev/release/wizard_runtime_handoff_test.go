package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWizardOffersClaudeOnboardingModalAfterWorkspaceCreation(t *testing.T) {
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
		`id="runtime-ready-modal"`,
		`data-action="launch-runtime" data-runtime="claude"`,
		"Abrir o Maestro no Claude Code Desktop",
		"guia de onboarding já instalado",
	} {
		if !strings.Contains(string(index), required) {
			t.Fatalf("wizard handoff modal is missing %q", required)
		}
	}
	for _, required := range []string{
		"runtimeReadyModal.showModal()",
		"runtimeReadyWorkspace.textContent = defaultWorkspace",
		"if (runtimeReadyModal?.open) runtimeReadyModal.close()",
	} {
		if !strings.Contains(string(app), required) {
			t.Fatalf("wizard handoff behavior is missing %q", required)
		}
	}
}

func TestWizardShowsClaudeModalOnlyForAClaudePreparedWorkspace(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "installers", "wizard", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{
		"let configuredRuntime = 'claude'",
		"preparedRuntime === 'claude'",
		"payload.activation?.lifecycle?.runtime || configuredRuntime",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("runtime-aware handoff is missing %q", required)
		}
	}
}
