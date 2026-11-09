package main

import (
	"crypto/sha256"
	"fmt"
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipmigration"
)

func TestProductInitCreatesAndPreservesLifecycleFilesystem(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	productRoot, user, work := filepath.Join(base, "product"), filepath.Join(base, "owner"), filepath.Join(base, "workspace")
	for _, path := range []string{productRoot, user} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("MAESTRO_HOME", filepath.Join(user, ".maestro"))
	prior := os.Args
	t.Cleanup(func() { os.Args = prior })
	os.Args = []string{"maestro-mcp", "init", "--product-root", productRoot, "--user-home", user, "--workspace", work}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	home, err := datahome.Resolve(datahome.Options{ProductRoot: productRoot, UserHome: user})
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := workspace.Inspect(work, filepath.Join(home.Home, "state"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(work, "brain"), filepath.Join(home.Home, "brain", "owner"),
		filepath.Join(home.Home, "brain", "memory", "workspaces", inspection.WorkspaceID, "l1", "captures"),
		filepath.Join(home.Home, "brain", "memory", "workspaces", inspection.WorkspaceID, "commits"),
		filepath.Join(home.Home, "state"),
	} {
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			t.Fatalf("explicit preparation did not create canonical directory %s: %v", path, err)
		}
	}
	if err := os.WriteFile(filepath.Join(work, "brain", "participant-note.md"), []byte("Synthetic authored work must survive repeated preparation.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot := func() map[string]string {
		t.Helper()
		files := map[string]string{}
		if err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(base, path)
			if err != nil {
				return err
			}
			if entry.IsDir() {
				files[relative] = info.Mode().String()
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files[relative] = fmt.Sprintf("%s:%x", info.Mode(), sha256.Sum256(body))
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return files
	}
	before := snapshot()
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("repeated explicit preparation changed canonical files, private keys, modes or authored work")
	}
}

func TestMigratedHomePublicMCPInit(t *testing.T) {
	if runtime.GOOS == "windows" && os.Getenv("MAESTRO_WINDOWS_CORE_PROOF") != "1" {
		t.Skip("dedicated ordinary-account Windows external migration proof required")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	user, source, productRoot, work := filepath.Join(base, "profile"), filepath.Join(base, "old ZIP"), filepath.Join(base, "product"), filepath.Join(base, "case")
	for _, dir := range []string{user, source, productRoot} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "VERSION"), []byte("0.1.12\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "data/profile"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "data/profile/identity.md"), []byte("synthetic owner\n"), 0600); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(user, ".maestro")
	t.Setenv("MAESTRO_HOME", home)
	p, err := zipmigration.PlanExternal(zipmigration.ExternalOptions{SourceRoot: source, ProductRoot: productRoot, UserHome: user, Home: home})
	if err != nil {
		t.Fatal(err)
	}
	body, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if s, err := zipmigration.ApplyExternal(p, fmt.Sprintf("%x", sha256.Sum256(body)), nil); err != nil || !s.Current {
		t.Fatalf("apply %+v %v", s, err)
	}
	h, err := datahome.Resolve(datahome.Options{ProductRoot: productRoot, UserHome: user})
	if err != nil || !h.Writable || h.Origin != "migrated" {
		t.Fatalf("resolve %+v %v", h, err)
	}
	origin := filepath.Join(home, "brain/.maestro/origin.json")
	before, err := os.ReadFile(origin)
	if err != nil {
		t.Fatal(err)
	}
	prior := os.Args
	defer func() { os.Args = prior }()
	os.Args = []string{"maestro-mcp", "init", "--product-root", productRoot, "--user-home", user, "--workspace", work}
	if err := run(); err != nil {
		t.Fatalf("admitted migrated HOME cannot enroll explicit workspace through public MCP init: %v", err)
	}
	after, err := os.ReadFile(origin)
	if err != nil || string(before) != string(after) {
		t.Fatal("migrated origin changed")
	}
	if _, err := product.Open(product.Options{ProductRoot: productRoot, UserHome: user, WorkspacePath: work}); err != nil {
		t.Fatalf("migrated workspace unavailable to MCP: %v", err)
	}
	for _, path := range []string{filepath.Join(source, "data/profile/identity.md"), filepath.Join(home, "brain/owner/identity.md")} {
		body, err := os.ReadFile(path)
		if err != nil || string(body) != "synthetic owner\n" {
			t.Fatalf("authored bytes changed: %s %q %v", path, body, err)
		}
	}
	foreignBrain := filepath.Join(productRoot, "brain")
	if err := os.MkdirAll(filepath.Join(foreignBrain, "owner"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreignBrain, "owner", "foreign.md"), []byte("unrelated owner"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := product.Open(product.Options{ProductRoot: productRoot, UserHome: user, WorkspacePath: work}); err == nil {
		t.Error("foreign legacy state admitted by MCP")
	}
	conflicted := filepath.Join(base, "conflicted case")
	os.Args[len(os.Args)-1] = conflicted
	if err := run(); err == nil {
		t.Error("foreign legacy state admitted for workspace init")
	}
	if _, err := os.Lstat(conflicted); !os.IsNotExist(err) {
		t.Error("conflicted init created workspace")
	}
	if err := os.RemoveAll(foreignBrain); err != nil {
		t.Fatal(err)
	}
	// Incomplete authority must never create a second workspace.
	if err := os.WriteFile(origin, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(base, "blocked case")
	os.Args[len(os.Args)-1] = blocked
	if err := run(); err == nil {
		t.Fatal("tampered authority admitted")
	}
	if _, err := os.Lstat(blocked); !os.IsNotExist(err) {
		t.Fatal("refused init created workspace")
	}

}
