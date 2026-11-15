//go:build !windows

package atlasops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestAuthoredNavigationPublicationKeepsLastCompleteGeneration(t *testing.T) {
	base := t.TempDir()
	registered, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(base, "case"), DataRoot: filepath.Join(base, "local")})
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	options := atlas.Options{DataRoot: registered.DataRoot, WorkspacePath: registered.WorkspacePath, WorkspaceID: registered.WorkspaceID, Now: func() time.Time { return clock }}
	engine, err := OpenWorkspace(options)
	if err != nil {
		t.Fatal(err)
	}
	source := func(page, marker string) string {
		return fmt.Sprintf("---\nid: %s\ntitle: Navigation source\nsummary: Explicit source.\ntype: project-brief\nscope: workspace/%s\nstatus: draft\nsensitivity: internal\nupdated: 2026-11-14\n---\n# Navigation source\n\n%s\n", strings.TrimSuffix(page, ".md"), registered.WorkspaceID, marker)
	}
	const a, b = "projects/a.md", "projects/b.md"
	for _, page := range []string{a, b} {
		if _, err = engine.CreatePage(CreatePageRequest{Page: page, Body: source(page, page), Provenance: attended(page)}); err != nil {
			t.Fatal(err)
		}
	}
	review, err := engine.ReviewNavigation([]string{a}, "Compile explicit A")
	if err != nil {
		t.Fatal(err)
	}
	first, err := engine.PublishNavigation(review, attended("first"))
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(engine.journalDirPath("navigation-current.json"))
	if err != nil {
		t.Fatal(err)
	}
	review, err = engine.ReviewNavigation([]string{a, b}, "Compile explicit A and B")
	if err != nil {
		t.Fatal(err)
	}
	interrupted := errors.New("stop after complete immutable generation before pointer")
	engine.afterPageEffect = func() error { return interrupted }
	if _, err = engine.PublishNavigation(review, attended("interrupted")); !errors.Is(err, interrupted) {
		t.Fatal("interruption not reached", err)
	}
	after, _ := os.ReadFile(engine.journalDirPath("navigation-current.json"))
	if string(after) != string(before) {
		t.Fatal("interruption replaced current pointer")
	}
	restarted, err := OpenWorkspace(options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.NavigationView(first.Generation); err != nil {
		t.Fatal("last good generation unavailable after cold restart", err)
	}
	clock = clock.Add(time.Hour)
	second, err := restarted.PublishNavigation(review, attended("explicit-retry"))
	if err != nil || second.State != StateWritten {
		t.Fatal("explicit staged retry failed", second, err)
	}
	if !second.PublishedAt.Equal(clock) || !second.PublishedAt.After(first.PublishedAt) {
		t.Fatal("staged time mislabeled as publication", first, second)
	}
	noOpReview, err := restarted.ReviewNavigation([]string{b, a}, "Check deterministic no-op")
	if err != nil {
		t.Fatal(err)
	}
	stableBefore, _ := os.ReadFile(engine.journalDirPath("navigation-current.json"))
	infoBefore, _ := os.Stat(engine.journalDirPath("navigation-current.json"))
	noOp, err := restarted.PublishNavigation(noOpReview, attended("no-op"))
	if err != nil || noOp.State != StateUnchanged {
		t.Fatal("no-op failed", noOp, err)
	}
	stableAfter, _ := os.ReadFile(engine.journalDirPath("navigation-current.json"))
	infoAfter, _ := os.Stat(engine.journalDirPath("navigation-current.json"))
	if string(stableBefore) != string(stableAfter) || !os.SameFile(infoBefore, infoAfter) {
		t.Fatal("no-op rewrote current pointer")
	}

	foreignRegistration, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(base, "foreign"), DataRoot: registered.DataRoot})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := OpenWorkspace(atlas.Options{DataRoot: foreignRegistration.DataRoot, WorkspacePath: foreignRegistration.WorkspacePath, WorkspaceID: foreignRegistration.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(foreign.journalDirPath(""), 0700); err != nil {
		t.Fatal(err)
	}
	generationName := "navigation-" + second.Generation + ".json"
	generationBytes, err := os.ReadFile(restarted.journalDirPath(generationName))
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{"navigation-current.json": stableAfter, generationName: generationBytes} {
		if err = os.WriteFile(foreign.journalDirPath(name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = foreign.NavigationView(second.Generation); err == nil {
		t.Fatal("copied generation crossed workspace scope")
	}
	sourcePath := filepath.Join(engine.root, filepath.FromSlash(b))
	outside := filepath.Join(base, "outside.md")
	if err = os.Rename(sourcePath, outside); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside, sourcePath); err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.NavigationView(second.Generation); !errors.Is(err, ErrNavigationStale) {
		t.Fatal("source alias remained visible", err)
	}
	if _, err = restarted.ReviewNavigation([]string{b}, "Do not follow source alias"); err == nil {
		t.Fatal("source alias was compiled")
	}
	if err = os.Remove(sourcePath); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(outside, sourcePath); err != nil {
		t.Fatal(err)
	}
	staleReview, _ := restarted.ReviewNavigation([]string{a}, "Review before source edit")
	if err = os.WriteFile(filepath.Join(engine.root, filepath.FromSlash(a)), []byte(source(a, "corrected")), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.PublishNavigation(staleReview, attended("stale")); !errors.Is(err, ErrReviewedTargetChanged) {
		t.Fatal("changed reviewed source was published", err)
	}
	if _, err = restarted.NavigationView(second.Generation); !errors.Is(err, ErrNavigationStale) {
		t.Fatal("edited source stayed visible", err)
	}
	if err = os.Remove(filepath.Join(engine.root, filepath.FromSlash(a))); err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.NavigationView(second.Generation); !errors.Is(err, ErrNavigationStale) {
		t.Fatal("retired source stayed visible", err)
	}
	if text, err := os.ReadFile(filepath.Join(engine.root, filepath.FromSlash(b))); err != nil || string(text) != source(b, b) {
		t.Fatal("compilation changed canonical B", err)
	}
}
