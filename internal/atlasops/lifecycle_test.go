package atlasops

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestPendingAuthoredLifecycleRetriesOnlyItsOwnedEffect(t *testing.T) {
	for _, operation := range []string{"move", "retire"} {
		t.Run(operation, func(t *testing.T) { testAuthoredLifecycleRecovery(t, operation, "renamed") })
	}
}

func TestAuthoredLifecycleDurableCutsRecoverExactOperation(t *testing.T) {
	for _, operation := range []string{"move", "retire"} {
		for _, phase := range []string{"prepared", "barrier", "lifecycle", "receipt"} {
			t.Run(operation+"/"+phase, func(t *testing.T) { testAuthoredLifecycleRecovery(t, operation, phase) })
		}
	}
}

func TestAuthoredLifecycleMetadataCutPreservesReviewedScope(t *testing.T) {
	testAuthoredLifecycleRecovery(t, "move", "metadata")
}

func testAuthoredLifecycleRecovery(t *testing.T, operation, phase string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("native lifecycle publication requires separate ordinary-user qualification")
	}
	base := t.TempDir()
	registered, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(base, "case"), DataRoot: filepath.Join(base, "local")})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)
	clock := started
	options := atlas.Options{DataRoot: registered.DataRoot, WorkspacePath: registered.WorkspacePath, WorkspaceID: registered.WorkspaceID, Now: func() time.Time { return clock }}
	engine, err := OpenWorkspace(options)
	if err != nil {
		t.Fatal(err)
	}
	const source, target = "projects/recovery.md", "accounts/a/cases/selected/recovered.md"
	body := "---\nid: projects/recovery\ntitle: Recovery proof\nsummary: Exact owned lifecycle recovery.\ntype: project-brief\nscope: workspace/" + registered.WorkspaceID + "\nstatus: draft\nsensitivity: internal\nupdated: 2026-11-08\n---\n# Recovery proof\n"
	created, err := engine.CreatePage(CreatePageRequest{Page: source, Body: body, Provenance: attended("create")})
	if err != nil {
		t.Fatal(err)
	}
	move := MovePageRequest{Page: source, Target: target, ExpectedRevision: created.Revision, Provenance: attended("move-once")}
	retire := RetirePageRequest{Page: source, ExpectedRevision: created.Revision, Provenance: attended("retire-once")}
	review := func(e *Engine) (LifecycleReview, error) {
		if operation == "retire" {
			return e.ReviewRetire(retire)
		}
		return e.ReviewMove(move)
	}
	apply := func(e *Engine) (Result, error) {
		if operation == "retire" {
			return e.RetirePage(retire)
		}
		return e.MovePage(move)
	}
	shown, err := review(engine)
	if err != nil {
		t.Fatal(err)
	}
	move.ReferencesDigest, retire.ReferencesDigest = shown.ReferencesDigest, shown.ReferencesDigest
	interrupted := errors.New("synthetic interruption after actual lifecycle rename")
	if phase == "renamed" {
		engine.afterPageEffect = func() error { return interrupted }
	} else if phase == "metadata" {
		effects := 0
		engine.afterPageEffect = func() error {
			effects++
			if effects == 2 {
				return interrupted
			}
			return nil
		}
	} else {
		engine.afterLifecyclePhase = func(actual string) error {
			if actual == phase {
				return interrupted
			}
			return nil
		}
	}
	if _, err := apply(engine); !errors.Is(err, interrupted) {
		t.Fatalf("did not reach actual rename: %v", err)
	}
	clock = clock.Add(24 * time.Hour)
	restarted, err := OpenWorkspace(options)
	if err != nil {
		t.Fatal(err)
	}
	before, err := restarted.LifecycleRevision()
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := restarted.ReviewPage(CollectRequest{Purpose: "Inspect pending lifecycle without recovery", Reader: ReaderOwnerSession, Pages: []string{source}})
	if (phase == "renamed" || phase == "metadata" || phase == "barrier") && !errors.Is(readErr, ErrLifecyclePending) {
		t.Fatalf("pending page disclosed: %v", readErr)
	}
	after, err := restarted.LifecycleRevision()
	if err != nil || before != after {
		t.Fatalf("read changed lifecycle revision: %d -> %d %v", before, after, err)
	}
	shown, err = review(restarted)
	if err != nil {
		t.Fatalf("exact owning review refused: %v", err)
	}
	move.ReferencesDigest, retire.ReferencesDigest = shown.ReferencesDigest, shown.ReferencesDigest
	recovered, err := apply(restarted)
	if err != nil {
		t.Fatalf("exact owning retry refused: %v", err)
	}
	if !recovered.RecordedAt.Equal(started) || recovered.LifecycleRevision != 2 {
		t.Fatalf("prepared receipt lost: %+v", recovered)
	}
	if operation == "retire" {
		archived, err := os.ReadFile(filepath.Join(engine.Root(), filepath.FromSlash(archivePage(recovered.OperationID))))
		if err != nil || string(archived) != body {
			t.Fatalf("archive lost exact source bytes: %v", err)
		}
	} else {
		current, err := restarted.ReviewPage(CollectRequest{Purpose: "Verify recovered current page", Reader: ReaderOwnerSession, Pages: []string{source}})
		if err != nil || current.Page != target || current.StableID != "projects/recovery" || current.Content != strings.Replace(strings.Replace(body, "scope: workspace/"+registered.WorkspaceID, `scope: "account/a/case/selected"`, 1), "sensitivity: internal", "sensitivity: client-confidential", 1) {
			t.Fatalf("recovered identity/content lost: %+v %v", current, err)
		}
	}
	if phase != "renamed" {
		replayed, err := apply(restarted)
		if err != nil || replayed.OperationID != recovered.OperationID || replayed.State != StateUnchanged {
			t.Fatalf("owning replay was not exactly once: %+v %v", replayed, err)
		}
	}
}

func TestAuthoredLifecycleRecoveryPreservesConflictingFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native lifecycle publication requires separate ordinary-user qualification")
	}
	for _, operation := range []string{"move", "retire"} {
		for _, phase := range []string{"prepared", "lifecycle"} {
			for _, conflict := range []string{"new-source", "new-target", "changed-request", "lost-admission"} {
				t.Run(operation+"/"+phase+"/"+conflict, func(t *testing.T) {
					base := t.TempDir()
					registered, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(base, "case"), DataRoot: filepath.Join(base, "local")})
					if err != nil {
						t.Fatal(err)
					}
					options := atlas.Options{DataRoot: registered.DataRoot, WorkspacePath: registered.WorkspacePath, WorkspaceID: registered.WorkspaceID}
					admitted := true
					policy := WorkspacePolicy{AdmitPage: func(string) error {
						if !admitted {
							return errors.New("selected scope no longer admitted")
						}
						return nil
					}}
					engine, err := OpenWorkspaceWithPolicy(options, policy)
					if err != nil {
						t.Fatal(err)
					}
					const source = "projects/conflict.md"
					body := "---\nid: projects/conflict\ntitle: Conflict proof\nsummary: Preserve newer lifecycle bytes.\ntype: project-brief\nscope: workspace/" + registered.WorkspaceID + "\nstatus: draft\nsensitivity: internal\nupdated: 2026-11-08\n---\n# Conflict proof\n"
					created, err := engine.CreatePage(CreatePageRequest{Page: source, Body: body, Provenance: attended("create")})
					if err != nil {
						t.Fatal(err)
					}
					move := MovePageRequest{Page: source, Target: "projects/current.md", ExpectedRevision: created.Revision, Provenance: attended("move-once")}
					retire := RetirePageRequest{Page: source, ExpectedRevision: created.Revision, Provenance: attended("retire-once")}
					review := func(e *Engine) (LifecycleReview, error) {
						if operation == "retire" {
							return e.ReviewRetire(retire)
						}
						return e.ReviewMove(move)
					}
					apply := func(e *Engine) (Result, error) {
						if operation == "retire" {
							return e.RetirePage(retire)
						}
						return e.MovePage(move)
					}
					shown, err := review(engine)
					if err != nil {
						t.Fatal(err)
					}
					move.ReferencesDigest, retire.ReferencesDigest = shown.ReferencesDigest, shown.ReferencesDigest
					interrupted := errors.New("synthetic durable cut")
					engine.afterLifecyclePhase = func(actual string) error {
						if actual == phase {
							return interrupted
						}
						return nil
					}
					if _, err := apply(engine); !errors.Is(err, interrupted) {
						t.Fatalf("did not reach actual durable cut: %v", err)
					}
					sourcePath, targetPath := filepath.Join(engine.Root(), source), filepath.Join(engine.Root(), filepath.FromSlash(shown.Target))
					switch conflict {
					case "new-source", "new-target":
						path := sourcePath
						if conflict == "new-target" {
							path = targetPath
						}
						if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(path, []byte("# Newer externally authored bytes\n"), 0600); err != nil {
							t.Fatal(err)
						}
					case "changed-request":
						move.ExpectedRevision, retire.ExpectedRevision = strings.Repeat("f", 64), strings.Repeat("f", 64)
					case "lost-admission":
						admitted = false
					}
					beforeSource, sourceErr := os.ReadFile(sourcePath)
					beforeTarget, targetErr := os.ReadFile(targetPath)
					restarted, err := OpenWorkspaceWithPolicy(options, policy)
					if err != nil {
						t.Fatal(err)
					}
					beforeRevision, err := restarted.LifecycleRevision()
					if err != nil {
						t.Fatal(err)
					}
					if _, err := review(restarted); err == nil {
						t.Fatal("conflicting recovery obtained a review")
					}
					if _, err := apply(restarted); err == nil {
						t.Fatal("conflicting recovery obtained a receipt")
					}
					afterRevision, err := restarted.LifecycleRevision()
					if err != nil || beforeRevision != afterRevision {
						t.Fatalf("refusal changed lifecycle revision: %d -> %d %v", beforeRevision, afterRevision, err)
					}
					afterSource, afterSourceErr := os.ReadFile(sourcePath)
					afterTarget, afterTargetErr := os.ReadFile(targetPath)
					if string(beforeSource) != string(afterSource) || string(beforeTarget) != string(afterTarget) || errors.Is(sourceErr, os.ErrNotExist) != errors.Is(afterSourceErr, os.ErrNotExist) || errors.Is(targetErr, os.ErrNotExist) != errors.Is(afterTargetErr, os.ErrNotExist) {
						t.Fatal("conflicting recovery modified or removed existing/newer files")
					}
				})
			}
		}
	}
}

func TestAuthoredLifecycleReadersRefuseABACohorts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native lifecycle publication requires separate ordinary-user qualification")
	}
	for _, operation := range []string{"review", "collect"} {
		t.Run(operation, func(t *testing.T) {
			base := t.TempDir()
			registered, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(base, "case"), DataRoot: filepath.Join(base, "local")})
			if err != nil {
				t.Fatal(err)
			}
			options := atlas.Options{DataRoot: registered.DataRoot, WorkspacePath: registered.WorkspacePath, WorkspaceID: registered.WorkspaceID}
			writer, err := OpenWorkspace(options)
			if err != nil {
				t.Fatal(err)
			}
			reader, err := OpenWorkspace(options)
			if err != nil {
				t.Fatal(err)
			}
			const source, target = "projects/reader.md", "projects/reader-away.md"
			body := "---\nid: projects/reader\ntitle: Reader proof\nsummary: Fence disclosure against lifecycle ABA.\ntype: project-brief\nscope: workspace/" + registered.WorkspaceID + "\nstatus: draft\nsensitivity: internal\nupdated: 2026-11-08\n---\n# Reader proof\n"
			created, err := writer.CreatePage(CreatePageRequest{Page: source, Body: body, Provenance: attended("create")})
			if err != nil {
				t.Fatal(err)
			}
			fired := false
			reader.beforeAuthoredDisclosure = func() {
				if fired {
					return
				}
				fired = true
				revision, pageRevision := uint64(0), created.Revision
				for index, destination := range []string{target, source} {
					page := source
					if index == 1 {
						page = target
					}
					request := MovePageRequest{Page: page, Target: destination, ExpectedRevision: pageRevision, ExpectedLifecycleRevision: revision, Provenance: attended(destination)}
					shown, err := writer.ReviewMove(request)
					if err != nil {
						t.Fatal(err)
					}
					request.ReferencesDigest = shown.ReferencesDigest
					moved, err := writer.MovePage(request)
					if err != nil {
						t.Fatal(err)
					}
					revision, pageRevision = moved.LifecycleRevision, moved.Revision
				}
			}
			request := CollectRequest{Purpose: "Read one coherent authored cohort", Reader: ReaderOwnerSession, Pages: []string{source}}
			if operation == "review" {
				_, err = reader.ReviewPage(request)
			} else {
				_, err = reader.Collect(request)
			}
			if !fired {
				t.Fatal("did not reach the actual disclosure cut")
			}
			if !errors.Is(err, ErrLifecycleChanged) {
				t.Fatalf("disclosed or re-stamped a pre-ABA body as a new cohort: %v", err)
			}
			current, err := writer.ReviewPage(request)
			if err != nil || current.Content != body || current.LifecycleRevision != 4 {
				t.Fatalf("ABA fixture did not preserve same body at a new lifecycle revision: %+v %v", current, err)
			}
		})
	}
}

func TestAuthoredLifecycleRecoveryReviewsEveryReservedAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native lifecycle publication requires separate ordinary-user qualification")
	}
	base := t.TempDir()
	registered, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(base, "case"), DataRoot: filepath.Join(base, "local")})
	if err != nil {
		t.Fatal(err)
	}
	options := atlas.Options{DataRoot: registered.DataRoot, WorkspacePath: registered.WorkspacePath, WorkspaceID: registered.WorkspaceID}
	engine, err := OpenWorkspace(options)
	if err != nil {
		t.Fatal(err)
	}
	const original, current, target, referring = "projects/original.md", "projects/current.md", "projects/next.md", "projects/referring.md"
	body := "---\nid: projects/original\ntitle: Alias reference proof\nsummary: Review links through every reserved name.\ntype: project-brief\nscope: workspace/" + registered.WorkspaceID + "\nstatus: draft\nsensitivity: internal\nupdated: 2026-11-08\n---\n# Alias reference proof\n"
	created, err := engine.CreatePage(CreatePageRequest{Page: original, Body: body, Provenance: attended("create")})
	if err != nil {
		t.Fatal(err)
	}
	first := MovePageRequest{Page: original, Target: current, ExpectedRevision: created.Revision, Provenance: attended("first")}
	shown, err := engine.ReviewMove(first)
	if err != nil {
		t.Fatal(err)
	}
	first.ReferencesDigest = shown.ReferencesDigest
	moved, err := engine.MovePage(first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.CreatePage(CreatePageRequest{Page: referring, Body: "# Incoming link\n\n[Old stable name](original.md)\n", Provenance: attended("reference")}); err != nil {
		t.Fatal(err)
	}
	second := MovePageRequest{Page: current, Target: target, ExpectedRevision: moved.Revision, ExpectedLifecycleRevision: moved.LifecycleRevision, Provenance: attended("second")}
	shown, err = engine.ReviewMove(second)
	if err != nil {
		t.Fatal(err)
	}
	second.ReferencesDigest = shown.ReferencesDigest
	interrupted := errors.New("synthetic interruption after actual second move")
	engine.afterPageEffect = func() error { return interrupted }
	if _, err := engine.MovePage(second); !errors.Is(err, interrupted) {
		t.Fatalf("did not reach actual second namespace effect: %v", err)
	}
	restarted, err := OpenWorkspace(options)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := restarted.ReviewMove(second)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, reference := range recovered.References {
		found = found || reference.Page == referring && reference.Target == original
	}
	if !found || recovered.ReferencesDigest != shown.ReferencesDigest {
		t.Fatalf("recovery omitted an affected reference through a former reserved alias: before=%+v after=%+v", shown.References, recovered.References)
	}
}
