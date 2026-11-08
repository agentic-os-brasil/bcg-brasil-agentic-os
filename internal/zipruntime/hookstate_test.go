package zipruntime_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipmigration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipruntime"
)

func TestHookOwnedAliasesBlockCanonicalAdmission(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	for _, tc := range []struct{ path, kind string }{
		{".maestro/context-state", "directory"},
		{".maestro/context-state/context-inject-fixture.marker", "file"},
		{".maestro/context-envelope.json", "file"},
		{".maestro/agent-runs.json.tmp", "file"},
		{".scaffold.log", "file"},
		{".maestro-python", "dangling"},
		{".recovered-old", "file"},
		{"owner/self", "directory"},
		{"owner/observations/observations.jsonl", "hardlink"},
		{"memory/policies/lifetime.json", "file"},
	} {
		t.Run(tc.path+"-"+tc.kind, func(t *testing.T) {
			base, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			user, product, outside := filepath.Join(base, "user"), filepath.Join(base, "core"), filepath.Join(base, "outside")
			for _, path := range []string{user, product, outside} {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			lookup := func(string) (string, bool) { return "", false }
			home, err := datahome.InitializeFresh(datahome.Options{UserHome: user, ProductRoot: product, LookupEnv: lookup})
			if err != nil {
				t.Fatal(err)
			}
			options := zipruntime.Options{UserHome: user, LookupEnv: lookup}
			if paths := zipruntime.ResolvePaths(product, options); !paths.Writable {
				t.Fatal(paths)
			}
			target := filepath.Join(outside, "sentinel")
			if err := os.WriteFile(target, []byte("preserve outside"), 0600); err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(home.BrainRoot, tc.path)
			if err := os.MkdirAll(filepath.Dir(alias), 0700); err != nil {
				t.Fatal(err)
			}
			switch tc.kind {
			case "directory":
				err = os.Symlink(product, alias)
			case "dangling":
				err = os.Symlink(filepath.Join(outside, "missing"), alias)
			case "hardlink":
				err = os.Link(target, alias)
			default:
				err = os.Symlink(target, alias)
			}
			if err != nil {
				t.Fatal(err)
			}
			paths := zipruntime.ResolvePaths(product, options)
			if paths.Writable || paths.Mode != "blocked" || paths.Diagnostic != "hook-state-unsafe" {
				t.Fatalf("hook alias admitted: %+v", paths)
			}
			body, err := json.Marshal(paths)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), "brain_root") || strings.Contains(string(body), "product_root") {
				t.Fatalf("blocked response exposed usable paths: %s", body)
			}
			var out bytes.Buffer
			if err := zipruntime.RunWithOptions([]string{"context-cap", "--root", product, "--max", "100"}, strings.NewReader("private packet"), &out, options); err == nil || out.Len() != 0 {
				t.Fatalf("unsafe hook state produced context/effects: %v %s", err, &out)
			}
			if body, err := os.ReadFile(target); err != nil || string(body) != "preserve outside" {
				t.Fatalf("outside changed: %s %v", body, err)
			}
		})
	}
}

func TestHookPreflightRetainsDailySourceRefusal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	user, product := filepath.Join(base, "user"), filepath.Join(base, "core")
	for _, path := range []string{user, product} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	lookup := func(string) (string, bool) { return "", false }
	home, err := datahome.InitializeFresh(datahome.Options{UserHome: user, ProductRoot: product, LookupEnv: lookup})
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(base, "outside.md")
	if err := os.WriteFile(source, []byte("outside-private-body"), 0600); err != nil {
		t.Fatal(err)
	}
	daily := filepath.Join(home.BrainRoot, "daily")
	if err := os.Mkdir(daily, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, filepath.Join(daily, time.Now().Format("2006-01-02")+".md")); err != nil {
		t.Fatal(err)
	}
	options := zipruntime.Options{UserHome: user, LookupEnv: lookup}
	if paths := zipruntime.ResolvePaths(product, options); !paths.Writable {
		t.Fatal(paths)
	}
	var out bytes.Buffer
	if err := zipruntime.RunWithOptions([]string{"daily-context", "--root", product}, strings.NewReader(""), &out, options); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "unavailable") || strings.Contains(out.String(), "outside-private-body") {
		t.Fatal(out.String())
	}
	if body, err := os.ReadFile(source); err != nil || string(body) != "outside-private-body" {
		t.Fatalf("outside changed: %s %v", body, err)
	}
}

func TestHookPreflightPreservesCommittedLegacyAndCaseAliases(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	user, product := filepath.Join(base, "user"), filepath.Join(base, "core")
	for _, path := range []string{user, filepath.Join(product, "data/profile")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(product, "data/profile/identity.md"), []byte("legacy owner"), 0600); err != nil {
		t.Fatal(err)
	}
	if result := zipmigration.Run(product, zipmigration.Options{}); result.State != "committed" {
		t.Fatal(result)
	}
	options := zipruntime.Options{UserHome: user, LookupEnv: func(string) (string, bool) { return "", false }}
	if paths := zipruntime.ResolvePaths(product, options); !paths.Writable || paths.Mode != "legacy" {
		t.Fatal(paths)
	}
	// An account/case alias belongs to guard policy, outside hook-owned paths.
	external := filepath.Join(base, "case workspace")
	if err := os.Mkdir(external, 0700); err != nil {
		t.Fatal(err)
	}
	// Legacy migration has its own retained-source policy; use a non-migrated
	// existing owner for the alias assertion so that independent policy stays intact.
	plain := filepath.Join(base, "existing core")
	if err := os.MkdirAll(filepath.Join(plain, "brain/accounts/a/cases"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(plain, "brain/accounts/a/cases/one")); err != nil {
		t.Skip("case symlinks unavailable:", err)
	}
	if paths := zipruntime.ResolvePaths(plain, options); !paths.Writable || paths.Mode != "legacy" {
		t.Fatal(paths)
	}
}
