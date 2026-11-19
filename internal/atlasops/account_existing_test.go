package atlasops_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestAccountNarrativeConstructorIsFixedAndNoncreating(t *testing.T) {
	home := atlasops.AtlasDataRootForTest(t)
	if _, err := atlasops.OpenAccountExisting(home, "synthetic", nil); err == nil {
		t.Fatal("missing brain accepted")
	}
	if _, err := os.Lstat(filepath.Join(home, "brain")); !os.IsNotExist(err) {
		t.Fatal("query created brain", err)
	}
	if err := scheduler.EnsurePrivateDirectory(filepath.Join(home, "brain")); err != nil {
		t.Fatal(err)
	}
	guardCalls := 0
	engine, err := atlasops.OpenAccountExisting(home, "synthetic", func() error { guardCalls++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range []string{"accounts/foreign/foreign.md", "accounts/synthetic/cases/first/first.md", "accounts/synthetic/../synthetic/synthetic.md", "daily/2026-11-16.md", "../outside.md"} {
		if _, err := engine.CreatePage(atlasops.CreatePageRequest{Page: page, Body: "not authorized", Provenance: provenance("foreign-account")}); err == nil {
			t.Fatal("noncanonical page admitted", page)
		}
	}
	if guardCalls != 0 {
		t.Fatal("outside pages reached publication guard")
	}
	page := "accounts/synthetic/synthetic.md"
	saved, err := engine.CreatePage(atlasops.CreatePageRequest{Page: page, Body: "# Account\n", Provenance: provenance("account-fixed")})
	if err != nil || saved.State != atlasops.StateWritten || guardCalls < 1 {
		t.Fatal(saved, guardCalls, err)
	}
	read, err := engine.ReviewPage(atlasops.CollectRequest{Pages: []string{page}, Purpose: "Review exact account page", Reader: atlasops.ReaderOwnerSession})
	if err != nil || read.Content != "# Account\n" || read.Pointer != "bcgos://atlas/account/synthetic/"+page {
		t.Fatal(read, err)
	}
	// Keep root identity, not just pathname equality, across every call.
	if err := os.Rename(filepath.Join(home, "brain"), filepath.Join(home, "old-brain")); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.EnsurePrivateDirectory(filepath.Join(home, "brain")); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ReviewPage(atlasops.CollectRequest{Pages: []string{page}, Purpose: "Refuse replaced root", Reader: atlasops.ReaderOwnerSession}); err == nil {
		t.Fatal("replaced root admitted")
	}
}
