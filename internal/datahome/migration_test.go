package datahome_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipmigration"
)

func TestResolveCommittedMigrationUsesSealedSourceIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	opts := fixture(t)
	source := filepath.Join(opts.UserHome, "old ZIP")
	for path, body := range map[string]string{"VERSION": "0.1.12\n", "data/profile/identity.md": "original owner"} {
		name := filepath.Join(source, path)
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := zipmigration.PlanExternal(zipmigration.ExternalOptions{SourceRoot: source, ProductRoot: opts.ProductRoot, UserHome: opts.UserHome, Home: filepath.Join(opts.UserHome, ".maestro")})
	if err != nil {
		t.Fatal(err)
	}
	body, err := plan.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	if status, err := zipmigration.ApplyExternal(plan, digest, nil); err != nil || !status.Current {
		t.Fatalf("apply: %+v %v", status, err)
	}
	got, err := datahome.Resolve(opts)
	if err != nil || !got.Writable || got.Origin != "migrated" || got.Home != plan.Home {
		t.Fatalf("committed HOME unavailable: %+v %v", got, err)
	}
	otherUser := filepath.Join(opts.UserHome, "other user")
	if err := os.Mkdir(otherUser, 0700); err != nil {
		t.Fatal(err)
	}
	locator, err := os.ReadFile(filepath.Join(opts.UserHome, ".maestro-home.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherUser, ".maestro-home.json"), locator, 0600); err != nil {
		t.Fatal(err)
	}
	otherOptions := opts
	otherOptions.UserHome = otherUser
	if got, err := datahome.Resolve(otherOptions); err == nil || got.Writable || got.Diagnostic != "migration-conflict" {
		t.Fatalf("copied locator transferred profile authority: %+v %v", got, err)
	}
	opts.SourceRoot = opts.ProductRoot
	if got, err := datahome.Resolve(opts); err == nil || got.Writable {
		t.Fatalf("unrelated source accepted: %+v %v", got, err)
	}
	opts.SourceRoot = ""
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	if got, err := datahome.Resolve(opts); err != nil || !got.Writable {
		t.Fatalf("offline historical source blocked HOME: %+v %v", got, err)
	}
}
