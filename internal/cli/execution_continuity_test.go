package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstalledRuntimeRoutesPortugueseWorkRequestToExecutionContinuity(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "BCGOS")
	workspacePath := filepath.Join(t.TempDir(), "maestro-workspace")
	var output bytes.Buffer
	resolve := func() (string, error) { return dataRoot, nil }
	if code := runInit([]string{workspacePath}, &output, &output, resolve); code != ExitOK {
		t.Fatal(output.String())
	}
	completeQuickOwnerOnboarding(t, dataRoot)
	output.Reset()
	if code := runAdapterWithDataRoot([]string{"install", "--runtime", "claude", workspacePath}, &output, &output, resolve); code != ExitOK {
		t.Fatalf("adapter install = %d %s", code, output.String())
	}

	output.Reset()
	prompt := `{"session_id":"session-continuity","prompt":"Quero preparar a versão 2 da recomendação da Aurora Mobility incorporando uma sensibilidade de impacto financeiro."}`
	if code := runHookWithInput([]string{"claude", "context-injection", "--adapter-source", "maestro", workspacePath}, strings.NewReader(prompt), &output, &output, resolve); code != ExitOK {
		t.Fatalf("context injection = %d %s", code, output.String())
	}
	if !strings.Contains(output.String(), "execution-continuity") || !strings.Contains(output.String(), "lexical_intent") {
		t.Fatalf("continuity method was not routed: %s", output.String())
	}
}
