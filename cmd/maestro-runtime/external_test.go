package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipruntime"
)

func TestExternalPlanCLIIsExplicitAndReadOnly(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source, product, user := filepath.Join(base, "old ZIP"), filepath.Join(base, "new product"), filepath.Join(base, "profile")
	for _, path := range []string{filepath.Join(source, "data/profile"), product, user} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for path, body := range map[string]string{
		filepath.Join(source, "VERSION"):                  "0.1.12\n",
		filepath.Join(source, "data/profile/identity.md"): "# Synthetic existing owner\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	home := filepath.Join(user, ".maestro")
	t.Setenv("MAESTRO_HOME", home)
	var out, diagnostic bytes.Buffer
	code := run([]string{"migration", "external", "plan", "--source", source, "--product-root", product, "--user-home", user}, strings.NewReader(""), &out, &diagnostic)
	if code != 0 {
		t.Fatalf("explicit external plan refused: exit=%d diagnostic=%s", code, &diagnostic)
	}
	var plan map[string]any
	if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
		t.Fatalf("reviewable plan JSON: %v; %s", err, &out)
	}
	if plan["source_root"] != source || plan["home"] != home {
		t.Fatalf("plan lost the operator binding: %v", plan)
	}
	digest := sha256.Sum256(out.Bytes())
	if !strings.Contains(diagnostic.String(), hex.EncodeToString(digest[:])) {
		t.Fatal("plan output does not bind the exact reviewed bytes")
	}
	for _, path := range []string{home, filepath.Join(user, ".maestro-home.json")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("read-only planning changed state: %s (%v)", path, err)
		}
	}
	if body, err := os.ReadFile(filepath.Join(source, "data/profile/identity.md")); err != nil || string(body) != "# Synthetic existing owner\n" {
		t.Fatalf("planning changed original author bytes: %q %v", body, err)
	}
}

func externalCLIPlan(t *testing.T, version string) (source, user, home string, body []byte) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source, user = filepath.Join(base, "source"), filepath.Join(base, "profile")
	product := filepath.Join(base, "product")
	for _, path := range []string{filepath.Join(source, "brain/owner"), product, user} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for path, value := range map[string]string{filepath.Join(source, "VERSION"): version + "\n", filepath.Join(source, "brain/owner/identity.md"): "original author\n"} {
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	home = filepath.Join(user, ".maestro")
	t.Setenv("MAESTRO_HOME", home)
	var out, diagnostic bytes.Buffer
	if code := run([]string{"migration", "external", "plan", "--source", source, "--product-root", product, "--user-home", user}, strings.NewReader(""), &out, &diagnostic); code != 0 {
		t.Fatalf("plan: %d %s", code, &diagnostic)
	}
	return source, user, home, append([]byte(nil), out.Bytes()...)
}

func TestExternalCLIRejectsUnreviewedPlanBeforeEffects(t *testing.T) {
	_, user, home, body := externalCLIPlan(t, "0.2.0")
	for _, test := range []struct {
		name   string
		body   []byte
		digest string
	}{
		{"wrong-digest", body, strings.Repeat("0", 64)},
		{"noncanonical-document", append(append([]byte(nil), body...), ' '), ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(user, test.name+".json")
			if err := os.WriteFile(path, test.body, 0600); err != nil {
				t.Fatal(err)
			}
			digest := test.digest
			if digest == "" {
				sum := sha256.Sum256(test.body)
				digest = hex.EncodeToString(sum[:])
			}
			var out, diagnostic bytes.Buffer
			if code := run([]string{"migration", "external", "apply", "--plan", path, "--plan-sha256", digest}, strings.NewReader(""), &out, &diagnostic); code != 2 || out.Len() != 0 {
				t.Fatalf("unreviewed bytes accepted: %d %s %s", code, &out, &diagnostic)
			}
			for _, target := range []string{home, filepath.Join(user, ".maestro-home.json")} {
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatalf("rejection changed target: %s %v", target, err)
				}
			}
		})
	}
}

func TestExternalCLIApplyStatusAndRollbackKeepBindingExplicit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external HOME mutation is unavailable")
	}
	source, user, home, body := externalCLIPlan(t, "0.2.0")
	planPath := filepath.Join(user, "reviewed-plan.json")
	if err := os.WriteFile(planPath, body, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])
	var out, diagnostic bytes.Buffer
	if code := run([]string{"migration", "external", "apply", "--plan", planPath, "--plan-sha256", digest}, strings.NewReader(""), &out, &diagnostic); code != 0 {
		t.Fatalf("apply: %d %s %s", code, &out, &diagnostic)
	}
	var applied map[string]any
	if err := json.Unmarshal(out.Bytes(), &applied); err != nil || applied["current"] != true {
		t.Fatalf("apply did not publish current proof: %v %s", err, &out)
	}
	attempt, ok := applied["attempt_id"].(string)
	if !ok {
		t.Fatal("missing attempt")
	}
	out.Reset()
	diagnostic.Reset()
	if code := run([]string{"migration", "external", "status", "--home", home, "--attempt", attempt, "--plan-sha256", digest}, strings.NewReader(""), &out, &diagnostic); code != 0 {
		t.Fatalf("status: %d %s %s", code, &out, &diagnostic)
	}
	if err := os.WriteFile(filepath.Join(home, "brain/owner/identity.md"), []byte("new HOME edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	diagnostic.Reset()
	if code := run([]string{"migration", "external", "rollback", "--home", home, "--attempt", attempt, "--plan-sha256", digest}, strings.NewReader(""), &out, &diagnostic); code != 2 {
		t.Fatalf("inherited binding was falsely restored: %d %s %s", code, &out, &diagnostic)
	}
	var rolled map[string]any
	if err := json.Unmarshal(out.Bytes(), &rolled); err != nil || rolled["diagnostic"] != "rolled_back_binding_pending" || rolled["restored_runtime"] != false {
		t.Fatalf("rollback truth: %v %s %s", err, &out, &diagnostic)
	}
	archive, ok := rolled["archive_root"].(string)
	if !ok {
		t.Fatal("missing retained HOME path")
	}
	if value, err := os.ReadFile(filepath.Join(archive, "brain/owner/identity.md")); err != nil || string(value) != "new HOME edit\n" {
		t.Fatalf("rollback lost new work: %q %v", value, err)
	}
	if os.Getenv("MAESTRO_HOME") != home {
		t.Fatal("rollback changed the caller environment")
	}
	previous := zipruntime.ResolvePaths(source, zipruntime.Options{UserHome: user, LookupEnv: func(string) (string, bool) { return "", false }})
	if !previous.Writable || previous.Mode != "legacy" {
		t.Fatalf("previous explicit binding unavailable: %+v", previous)
	}
}
