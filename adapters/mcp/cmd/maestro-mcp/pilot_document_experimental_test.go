//go:build maestro_experimental_installed_seed

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/pilotdocument"
)

func TestInstalledPilotDocumentExclusiveFixedFixtureAndCollisionPreservation(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, pilotdocument.Name)
	marker := filepath.Join(root, "owner.txt")
	if err := os.WriteFile(marker, []byte("preserve existing workspace"), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := prepareFixedPilotDocument(root)
	if err != nil || state != "fixture_prepared" {
		t.Fatalf("public fixture prepare: %s %v", state, err)
	}
	expected, err := pilotdocument.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(body, expected) {
		t.Fatal("fixed public fixture bytes differ")
	}
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	state, err = prepareFixedPilotDocument(root)
	if err != nil || state != "fixture_already_present" {
		t.Fatalf("idempotent public verification: %s %v", state, err)
	}
	after, err := os.Stat(target)
	if err != nil || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) || !os.SameFile(before, after) {
		t.Fatal("identical existing file was modified")
	}
	if body, err := os.ReadFile(marker); err != nil || string(body) != "preserve existing workspace" {
		t.Fatal("public preparation changed workspace files")
	}
	for _, kind := range []string{"different", "directory", "symlink", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			work := t.TempDir()
			destination := filepath.Join(work, pilotdocument.Name)
			outside := filepath.Join(t.TempDir(), "outside.docx")
			if err := os.WriteFile(outside, expected, 0600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "different":
				if err := os.WriteFile(destination, []byte("existing user document"), 0644); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(destination, 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(outside, destination); err != nil {
					t.Skipf("symlink fixture unavailable: %v", err)
				}
			case "hardlink":
				if err := os.Link(outside, destination); err != nil {
					t.Skipf("hardlink fixture unavailable: %v", err)
				}
			}
			before, err := os.Lstat(destination)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := prepareFixedPilotDocument(work); err == nil {
				t.Fatal("collision was repaired, replaced or admitted")
			}
			after, err := os.Lstat(destination)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatal("collision entry changed")
			}
			if body, err := os.ReadFile(outside); err != nil || !bytes.Equal(body, expected) {
				t.Fatal("outside source changed")
			}
			if kind == "different" {
				if body, err := os.ReadFile(destination); err != nil || string(body) != "existing user document" {
					t.Fatal("user document overwritten")
				}
			}
		})
	}
}

func TestInstalledPilotPrepareDocumentRefusesReadOnlyAndForeignInputs(t *testing.T) {
	root := t.TempDir()
	args := []string{"--product-root", filepath.Join(root, "product"), "--user-home", filepath.Join(root, "owner"), "--workspace", filepath.Join(root, "work"), "--carrier-id", strings.Repeat("a", 32), "--installer-version", "0.1.51", "--experimental-seed-sha256", strings.Repeat("b", 64), "--experimental-installed-root", filepath.Join(root, "product")}
	for _, input := range [][]string{nil, append(append([]string{}, args...), "--check-only"), append(append([]string{}, args...), "--destination", root), args} {
		var output bytes.Buffer
		if err := runInstalledPilotPrepareDocument(context.Background(), input, &output); err == nil {
			t.Fatal("unadmitted preparation accepted")
		}
		var report pilotDocumentPreparation
		if err := json.Unmarshal(output.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if report.State != "fixture_not_prepared" || report.Conversion != "not_tested" || report.NativeQualification != "not_tested" {
			t.Fatalf("refusal promoted preparation to qualification: %+v", report)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "work")); !os.IsNotExist(err) {
		t.Fatal("unadmitted writer initialized workspace")
	}
}
