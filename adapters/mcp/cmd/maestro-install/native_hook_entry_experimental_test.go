//go:build maestro_experimental_installed_seed

package main

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

func TestExperimentalNativeEntryRefusesMissingIndependentSeed(t *testing.T) {
	if handled, err := experimentalNativeHookCommand(context.Background(), "stable-native-hook", "not-executed", nil, nil); !handled || err == nil {
		t.Fatal("native route did not refuse missing seed")
	}
	if handled, err := experimentalNativeHookCommand(context.Background(), "unrelated", "not-executed", nil, nil); handled || err != nil {
		t.Fatal("native route consumed unrelated command")
	}
}

func TestCompiledStableNativeHookAdmissionFailurePreservesEventExit(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "maestro-install")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-mod=readonly", "-tags=maestro_experimental_installed_seed", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compile actual installer entry: %v: %s", err, output)
	}
	// This exact entry has no signed installed seed. Its deliberate admission
	// failure must still use the selected hook's host blocking semantics and
	// body-free diagnostic, instead of main collapsing every error to exit 1.
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "SubagentStart", "SubagentStop", "Stop"} {
		t.Run(event, func(t *testing.T) {
			child := exec.Command(binary, "stable-native-hook", "--event", event)
			child.Stdin = strings.NewReader("PRIVATE_PAYLOAD_MUST_NOT_BE_LOGGED")
			var output, diagnostic bytes.Buffer
			child.Stdout, child.Stderr = &output, &diagnostic
			err := child.Run()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != product.HookExitCode(event) || output.Len() != 0 || strings.TrimSpace(diagnostic.String()) != "maestro: hook unavailable; inspect admitted state before retrying" {
				t.Fatalf("hook %s admission failure lost event exit/masking: exit=%v stdout=%s stderr=%s", event, err, output.String(), diagnostic.String())
			}
		})
	}
}
