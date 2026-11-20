//go:build windows

package docling

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
)

func testWindowsPlacementPublishesRetainedStageWithoutReplacement(t *testing.T) {
	for _, test := range []string{"publish", "occupied", "other_root_held"} {
		t.Run(test, func(t *testing.T) {
			private := filepath.Join(t.TempDir(), "private")
			directory, err := winprivate.Fresh(private)
			if err != nil {
				t.Fatal(err)
			}
			if err := directory.Close(); err != nil {
				t.Fatal(err)
			}
			parent, err := scheduler.HoldDirectory(private, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer parent.Close()
			stage := filepath.Join(private, ".placing-test")
			if err := os.Mkdir(stage, 0700); err != nil {
				t.Fatal(err)
			}
			payload := []byte("authenticated staged bytes")
			if err := os.WriteFile(filepath.Join(stage, "payload"), payload, 0600); err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(stage)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			original, err := root.Stat(".")
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(private, "installed-pack")
			var occupied os.FileInfo
			if test == "occupied" {
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(target, "payload"), []byte("prior pack"), 0600); err != nil {
					t.Fatal(err)
				}
				prior, err := winprivate.Open(target, false)
				if err != nil {
					t.Fatal(err)
				}
				occupied, err = prior.File.Stat()
				closeErr := prior.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("prior target identity: stat=%v close=%v", err, closeErr)
				}
			}
			if test == "other_root_held" {
				other, err := os.OpenRoot(stage)
				if err != nil {
					t.Fatal(err)
				}
				defer other.Close()
			}
			published, err := publishInstalledStage(root, parent, filepath.Base(target))
			if test == "publish" {
				if !published || err != nil {
					t.Fatalf("retained stage publication: published=%t err=%v", published, err)
				}
				if _, err := os.Stat(stage); !os.IsNotExist(err) {
					t.Fatalf("published stage name remains: %v", err)
				}
				stage = target
			} else if published || err == nil {
				t.Fatalf("unsafe publication accepted: published=%t err=%v", published, err)
			}
			// Compare against the held-root identity, not just the payload bytes.
			guard, openErr := winprivate.Open(stage, false)
			if openErr != nil {
				t.Fatal(openErr)
			}
			current, statErr := guard.File.Stat()
			closeErr := guard.Close()
			if statErr != nil || closeErr != nil || !os.SameFile(original, current) {
				t.Fatalf("staging identity was lost: stat=%v close=%v", statErr, closeErr)
			}
			body, readErr := os.ReadFile(filepath.Join(stage, "payload"))
			if readErr != nil || !bytes.Equal(body, payload) {
				t.Fatalf("staged bytes changed: %v", readErr)
			}
			if test == "occupied" {
				if !errors.Is(err, os.ErrExist) {
					t.Fatalf("expected exclusive publication refusal: %v", err)
				}
				current, err := os.Stat(target)
				body, readErr := os.ReadFile(filepath.Join(target, "payload"))
				if err != nil || !os.SameFile(occupied, current) || readErr != nil || string(body) != "prior pack" {
					t.Fatalf("existing pack was not preserved: stat=%v read=%v", err, readErr)
				}
			}
			if test == "other_root_held" {
				if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
					t.Fatalf("independent root custody was loosened: %v", err)
				}
				if _, err := os.Stat(target); !os.IsNotExist(err) {
					t.Fatalf("failed admission published a target: %v", err)
				}
			}
		})
	}
}
