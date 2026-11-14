package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// Exercise the actual Windows preparer inside the existing ordinary-account journey.
func assertWindowsPilotPreparation(t *testing.T, binary string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		return
	}
	source := filepath.Join("..", "..", "..", "..", "dev", "pilot")
	kit := t.TempDir()
	files := map[string]string{
		"Preparar Maestro.ps1":       "setup-windows.ps1",
		"templates/test-windows.ps1": "test-windows.ps1",
		"templates/test-windows.cmd": "test-windows.cmd",
	}
	for destination, original := range files {
		body, err := os.ReadFile(filepath.Join(source, original))
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(destination, ".ps1") && !bytes.HasPrefix(body, []byte{0xef, 0xbb, 0xbf}) {
			body = append([]byte{0xef, 0xbb, 0xbf}, body...)
		}
		writePilotFixture(t, filepath.Join(kit, destination), body)
	}
	body, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	writePilotFixture(t, filepath.Join(kit, "bin", "maestro-mcp-windows-amd64.exe"), body)
	writePilotFixture(t, filepath.Join(kit, "LIFECYCLE-CONTEXT.txt"), []byte("enabled\n"))
	var rows []string
	err = filepath.WalkDir(kit, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		relative, relErr := filepath.Rel(kit, path)
		if relErr != nil {
			return relErr
		}
		rows = append(rows, fmt.Sprintf("%x  ./%s\n", sha256.Sum256(body), filepath.ToSlash(relative)))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(rows)
	inventory := []byte(strings.Join(rows, ""))
	writePilotFixture(t, filepath.Join(kit, "SHA256SUMS"), inventory)
	writePilotFixture(t, filepath.Join(kit, "KIT-ID.txt"), []byte(fmt.Sprintf("%x\n", sha256.Sum256(inventory))))
	// The ordinary-account runner can inherit the administrator's profile variables.
	profile := t.TempDir()
	developer := filepath.Join(profile, "Developer")
	if err := os.MkdirAll(developer, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(developer, "MaestroPilot-selfserve-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	prepare := filepath.Join(kit, "Preparar Maestro.ps1")
	review := runPilotPowerShell(t, profile, prepare, "-Root", root, "-Review")
	match := regexp.MustCompile(`review_digest=([a-f0-9]{64})`).FindSubmatch(review)
	if len(match) != 2 {
		t.Fatal("actual preparer did not produce one review digest")
	}
	runPilotPowerShell(t, profile, prepare, "-Root", root, "-ConfirmDigest", string(match[1]))
	before, err := os.ReadFile(filepath.Join(root, "managed-files.sha256"))
	if err != nil {
		t.Fatal("actual preparer did not finish managed inventory:", err)
	}
	runPilotPowerShell(t, profile, filepath.Join(root, "launchers", "Testar Maestro.ps1"), "-CheckOnly")
	runPilotPowerShell(t, profile, prepare, "-Root", root, "-ConfirmDigest", string(match[1]))
	after, err := os.ReadFile(filepath.Join(root, "managed-files.sha256"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("self-check or repeated preparation changed managed inventory")
	}
	paths, err := filepath.Glob(filepath.Join(root, "diagnostics", "*", "report.json"))
	if err != nil || len(paths) != 3 {
		t.Fatal("preparation and emitted self-check must each retain a report")
	}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		var report struct {
			Status      string `json:"status"`
			WorkspaceID string `json:"workspace_id"`
			Markdown    string `json:"markdown"`
		}
		if err != nil || json.Unmarshal(body, &report) != nil || report.Status == "blocked" || report.WorkspaceID == "" {
			t.Fatal("actual Windows preparer or emitted self-check is blocked")
		}
		if !strings.Contains(report.Markdown, "diagnóstico do ensaio") {
			t.Fatal("Windows PowerShell lost UTF-8 report content")
		}
	}
}

func writePilotFixture(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
}

func runPilotPowerShell(t *testing.T, profile, script string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	arguments := append([]string{"-NoProfile", "-NonInteractive", "-File", script}, args...)
	cmd := exec.CommandContext(ctx, "powershell.exe", arguments...)
	for _, entry := range os.Environ() {
		key := strings.ToUpper(strings.SplitN(entry, "=", 2)[0])
		if key != "MAESTRO_HOME" && key != "USERPROFILE" && key != "HOME" {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "USERPROFILE="+profile, "HOME="+profile)
	body, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("actual Windows pilot script %s failed: %v\n%s", filepath.Base(script), err, body)
	}
	return body
}
