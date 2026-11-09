//go:build windows

package product_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
)

func windowsWikiDescriptor(t *testing.T, path string) string {
	t.Helper()
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return descriptor.String()
}

func windowsPrivateWikiCustody(t *testing.T, f ownerFixture, id, generation, concept, marker string, capture map[string]any) {
	t.Helper()
	policy, err := basememory.Policy()
	if err != nil {
		t.Fatal(err)
	}
	engine := memory.Engine{Root: filepath.Join(f.root, "memory"), Policy: policy, Now: time.Now, Budgets: map[string]int{"L1": 991, "L2": 997, "L3": 1009, "lifetime": 1013}}
	engine.Synthesizer = memory.DeterministicL1Synthesizer{MaxRunes: 4096, MaxEntries: 64, MaxInputBytes: 1 << 20, MaxInputEntries: 256, Attestor: memory.CaptureAttestor{Root: engine.Root}}
	engine.SynthesizerID = memory.DeterministicL1SynthesizerID
	root := filepath.Join(engine.Root, "workspaces", id, ".private-wiki")
	interrupted := errors.New("native private wiki deliberate interruption")
	for _, schedule := range []struct{ point, path string }{
		{"wiki_after_snapshot", f.options.UserHome},
		{"wiki_after_stage", filepath.Join(root, ".staging")},
		{"wiki_after_version_file", filepath.Join(root, "versions")},
		{"wiki_before_pointer", root},
	} {
		t.Run(schedule.point, func(t *testing.T) {
			probe := schedule.path + "-rename-probe"
			descriptor := windowsWikiDescriptor(t, schedule.path)
			if err := os.Rename(schedule.path, probe); err != nil {
				t.Fatal("rename baseline was not actually permitted", err)
			}
			if err := os.Rename(probe, schedule.path); err != nil {
				t.Fatal(err)
			}
			observed := false
			engine.FaultPoint = func(point string) error {
				if point != schedule.point {
					return nil
				}
				observed = true
				if err := os.Rename(schedule.path, probe); err == nil {
					if restoreErr := os.Rename(probe, schedule.path); restoreErr != nil {
						t.Fatal("restore after unsafe rename", restoreErr)
					}
					t.Fatal("retained graph transaction allowed namespace replacement")
				} else if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
					t.Fatal("namespace refusal was not native sharing/access denial", err)
				}
				return interrupted
			}
			if _, err := engine.CompilePrivateWiki(context.Background(), id, generation); !errors.Is(err, interrupted) || !observed {
				t.Fatal("native publication did not reach the retained transition", observed, err)
			}
			engine.FaultPoint = nil
			if windowsWikiDescriptor(t, schedule.path) != descriptor {
				t.Fatal("namespace custody changed the original ACL or owner")
			}
			page, err := engine.ReadPrivateWiki(id, generation, concept, 0, 65536)
			if err != nil || !strings.Contains(page.Content, marker) {
				t.Fatal("interruption changed the previous complete view", err)
			}
		})
	}
	engine.FaultPoint = nil
	compiled, err := engine.CompilePrivateWiki(context.Background(), id, generation)
	if err != nil || !compiled.Changed {
		t.Fatal("native interrupted publication could not resume", compiled, err)
	}
	assertDenied := func(generation, concept string) {
		t.Helper()
		if page, err := engine.ReadPrivateWiki(id, generation, concept, 0, 65536); err == nil || page.Content != "" {
			t.Fatal("denied native generation remained readable")
		}
	}
	finishRetirement := func() {
		t.Helper()
		_, err := engine.CompilePrivateWiki(context.Background(), id, "")
		if errors.Is(err, memory.ErrPrivateWikiRetirement) {
			t.Fatal("explicit retirement retry stayed pending", err)
		}
		if _, err = os.Lstat(root); !os.IsNotExist(err) {
			t.Fatal("explicit retry retained derived bodies", err)
		}
		status, err := engine.InspectPrivateWikiMaintenance(id)
		if err != nil || !status.RetirementComplete || status.State != "unavailable" {
			t.Fatal("retirement did not preserve denied source metadata", status, err)
		}
	}
	t.Run("sharing_blocks_retirement_but_not_denial", func(t *testing.T) {
		held, err := winprivate.OpenPinnedTree(root)
		if err != nil {
			t.Fatal(err)
		}
		defer held.Close()
		removed, err := engine.DeleteCapture(id, capture["id"].(string), uint64(capture["revision"].(float64)))
		if !errors.Is(err, memory.ErrPrivateWikiRetirement) || removed.Complete {
			t.Fatal("conflicting graph reader did not report pending retirement", removed, err)
		}
		assertDenied(compiled.Generation, concept)
		status, err := engine.InspectPrivateWikiMaintenance(id)
		if err != nil || status.RetirementComplete || status.State != "retirement_pending" {
			t.Fatal("pending retirement granted a view", status, err)
		}
		if err = held.Close(); err != nil {
			t.Fatal(err)
		}
		finishRetirement()
	})
	t.Run("hardlink_retirement_preserves_outside_bytes", func(t *testing.T) {
		at := time.Now().UTC()
		input, err := engine.SealOwnedCapture(id, "", 0, "WINDOWS_PRIVATE_WIKI_HARDLINK reviewed synthetic body", at)
		if err != nil {
			t.Fatal(err)
		}
		capture, err := engine.CaptureOwned(id, input)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = engine.DreamDailyAttested(context.Background(), id, at); err != nil {
			t.Fatal(err)
		}
		graph, err := engine.CompilePrivateWiki(context.Background(), id, "")
		if err != nil {
			t.Fatal(err)
		}
		index, err := engine.ReadPrivateWiki(id, graph.Generation, "index.md", 0, 65536)
		if err != nil {
			t.Fatal(err)
		}
		link := regexp.MustCompile(`\((concepts/[a-f0-9]{64}\.md)\)`).FindStringSubmatch(index.Content)
		if len(link) != 2 {
			t.Fatal("native hardlink control has no actual concept")
		}
		page := filepath.Join(root, "versions", graph.Generation, filepath.FromSlash(link[1]))
		outside := filepath.Join(filepath.Dir(f.options.ProductRoot), "outside-private-wiki.md")
		if err = os.Link(page, outside); err != nil {
			t.Fatal("actual ordinary-user hardlink fixture unavailable", err)
		}
		before, err := os.ReadFile(outside)
		if err != nil {
			t.Fatal(err)
		}
		descriptor := windowsWikiDescriptor(t, outside)
		removed, err := engine.DeleteCapture(id, capture.ID, capture.Revision)
		if !errors.Is(err, memory.ErrPrivateWikiRetirement) || removed.Complete {
			t.Fatal("hardlink cleanup did not report incomplete retirement", removed, err)
		}
		after, err := os.ReadFile(outside)
		if err != nil || string(after) != string(before) || windowsWikiDescriptor(t, outside) != descriptor {
			t.Fatal("retirement changed outside bytes or ACL", err)
		}
		assertDenied(graph.Generation, link[1])
		if err = os.Remove(outside); err != nil {
			t.Fatal(err)
		}
		finishRetirement()
	})
	t.Log("native_private_wiki_custody ancestor_rename_baseline=true retained_snapshot_and_publication_refuse_rename=true original_acl_preserved=true interrupted_retry=true sharing_retirement_pending=true denial_hides_old_graph=true hardlink_outside_bytes_preserved=true explicit_retirement_retry=true")
}
