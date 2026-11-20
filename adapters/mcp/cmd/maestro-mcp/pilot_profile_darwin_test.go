package main

import (
	"bytes"
	"context"
	"crypto/sha256"
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

func TestMacOSPilotSeedsOnlyNewCodeProfile(t *testing.T) {
	source := filepath.Join("..", "..", "..", "..", "dev", "pilot")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	temporary := filepath.Join(home, "Developer", "tmp")
	for _, directory := range []string{filepath.Join(home, "Developer"), temporary} {
		if err := os.Mkdir(directory, 0700); err != nil && !os.IsExist(err) {
			t.Fatal(err)
		}
		if info, err := os.Lstat(directory); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatal("fixture parent must be a physical Developer directory", err)
		}
	}
	base, err := os.MkdirTemp(temporary, "pilot-profile-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	root, kit := filepath.Join(base, "prepared space"), filepath.Join(base, "kit")
	binary := []byte("#!/bin/bash\nif [[ $1 == init ]]; then\n while [[ $# -gt 0 ]]; do if [[ $1 == --workspace ]]; then mkdir \"$2\"; break; fi; shift; done\n printf '%s\\n' '{\"state\":\"ready\",\"workspace_id\":\"synthetic-profile-fixture\"}'\nfi\n")
	image := "maestro-mcp-darwin-" + runtime.GOARCH
	files := map[string][]byte{"bin/" + image: binary, "LIFECYCLE-CONTEXT.txt": []byte("disabled\n")}
	setup, err := os.ReadFile(filepath.Join(source, "setup-macos.command"))
	if err != nil {
		t.Fatal(err)
	}
	files["Preparar Maestro.command"] = setup
	for _, name := range []string{"open-code-macos.command", "open-desktop-macos.command", "test-macos.command"} {
		files["templates/"+name] = []byte("#!/bin/bash\nexit 0\n")
	}
	rows := []string{}
	for name, body := range files {
		path := filepath.Join(kit, name)
		writePilotFixture(t, path, body)
		if err := os.Chmod(path, 0700); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, fmt.Sprintf("%x  ./%s\n", sha256.Sum256(body), name))
	}
	sort.Strings(rows)
	inventory := []byte(strings.Join(rows, ""))
	writePilotFixture(t, filepath.Join(kit, "SHA256SUMS"), inventory)
	writePilotFixture(t, filepath.Join(kit, "KIT-ID.txt"), []byte(fmt.Sprintf("%x\n", sha256.Sum256(inventory))))
	prepare := filepath.Join(kit, "Preparar Maestro.command")
	run := func(args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/bin/bash", append([]string{prepare, root}, args...)...)
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "MAESTRO_HOME=") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("actual preparer: %v\n%s", err, out)
		}
		return out
	}
	review := run("--review")
	match := regexp.MustCompile(`review_digest=([a-f0-9]{64})`).FindSubmatch(review)
	if len(match) != 2 {
		t.Fatal("review digest absent")
	}
	if !bytes.Contains(review, []byte("autoMemoryEnabled:false")) || !bytes.Contains(review, []byte("syncClaudeAiPlugins:false")) {
		t.Fatal("review did not disclose the scoped Code settings")
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("review created the target")
	}
	run("--confirm", string(match[1]))
	settings := filepath.Join(root, "client", "code", "settings.json")
	assertPilotFreshCodeSettings(t, settings)
	managed, err := os.ReadFile(filepath.Join(root, "managed-files.sha256"))
	if err != nil || bytes.Contains(managed, []byte("client/")) {
		t.Fatal("private Code settings entered managed inventory", err)
	}
	custom := []byte("{\"autoMemoryEnabled\":true,\"syncClaudeAiPlugins\":true,\"custom\":\"preserve\"}\n")
	if err := os.WriteFile(settings, custom, 0600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "client", "code", "customization.txt")
	writePilotFixture(t, marker, []byte("existing session fixture\n"))
	run("--confirm", string(match[1]))
	got, err := os.ReadFile(settings)
	if err != nil || !bytes.Equal(got, custom) {
		t.Fatal("repeat preparation rewrote existing settings", err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "existing session fixture\n" {
		t.Fatal("repeat preparation changed existing profile data", err)
	}
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	run("--confirm", string(match[1]))
	if _, err := os.Lstat(settings); !os.IsNotExist(err) {
		t.Fatal("repeat preparation silently created settings in an existing profile")
	}
}
