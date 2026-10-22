// Package spike is an isolated, synthetic-only adapter over Maestro's real
// execution ledger. It is not a production MCP or a data-home implementation.
package spike

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userlevel"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

const markerName = ".maestro-mcp-spike.json"

func developerRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return "", errors.New("current user's absolute home directory unavailable")
	}
	return filepath.Join(home, "Developer"), nil
}

type FixtureCase struct {
	WorkspaceID string `json:"workspace_id"`
	ItemID      string `json:"item_id"`
}

type Fixture struct {
	SchemaVersion int                    `json:"schema_version"`
	Purpose       string                 `json:"purpose"`
	Root          string                 `json:"root"`
	Cases         map[string]FixtureCase `json:"cases"`
}

type ContextOutput struct {
	Workspace     string   `json:"workspace"`
	ItemID        string   `json:"item_id"`
	State         string   `json:"state"`
	StateRevision int      `json:"state_revision"`
	AttemptID     string   `json:"attempt_id,omitempty"`
	Summary       string   `json:"summary,omitempty"`
	NextStep      string   `json:"next_step"`
	Blocker       string   `json:"blocker,omitempty"`
	ArtifactRefs  []string `json:"artifact_refs,omitempty"`
}

type CheckpointInput struct {
	ExpectedRevision int    `json:"expected_revision" jsonschema:"current state_revision from maestro_context"`
	AttemptID        string `json:"attempt_id" jsonschema:"current attempt_id from maestro_context"`
	Summary          string `json:"summary" jsonschema:"synthetic handoff summary, at most 768 UTF-8 bytes"`
	NextStep         string `json:"next_step" jsonschema:"specific next action, at most 768 UTF-8 bytes"`
	Pause            bool   `json:"pause" jsonschema:"pause after saving so a later process can resume"`
}

type ResumeInput struct {
	ExpectedRevision int `json:"expected_revision" jsonschema:"current paused state_revision from maestro_context"`
}

type CheckpointOutput struct {
	Workspace     string `json:"workspace"`
	State         string `json:"state"`
	StateRevision int    `json:"state_revision"`
	AttemptID     string `json:"attempt_id,omitempty"`
	CheckpointID  string `json:"checkpoint_id,omitempty"`
}

// Seed creates exactly two synthetic workspaces in a previously absent root.
// A failed seed is deliberately left for inspection and is never overwritten.
func Seed(root string) (Fixture, error) {
	if err := safeRoot(root); err != nil {
		return Fixture{}, err
	}
	if err := userlevel.EnsureNotElevated(); err != nil {
		return Fixture{}, err
	}
	if err := os.Mkdir(root, 0700); err != nil {
		return Fixture{}, fmt.Errorf("seed requires a new directory: %w", err)
	}
	fixture := Fixture{SchemaVersion: 1, Purpose: "maestro-mcp-synthetic-only", Root: root, Cases: map[string]FixtureCase{}}
	store := execution.Store{Root: filepath.Join(root, "state")}
	for _, name := range []string{"case-a", "case-b"} {
		ws, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(root, name), DataRoot: store.Root})
		if err != nil {
			return Fixture{}, fmt.Errorf("seed %s: %w", name, err)
		}
		item, err := store.Create(execution.CreateInput{
			WorkspaceID:     ws.WorkspaceID,
			Objective:       "Prepare the synthetic recommendation for " + name + ".",
			InitialNextStep: "Inspect the synthetic evidence for " + name + " before recommending.",
			Criteria:        []execution.Criterion{{ID: "recommendation", Type: execution.CriterionArtifactSnapshot, TargetRef: "bcgos://workspace/recommendation"}},
			AllowedRefs:     []string{"bcgos://workspace/recommendation"},
		})
		if err != nil {
			return Fixture{}, err
		}
		if _, err := store.Start(ws.WorkspaceID, item.Contract.ItemID, item.State.StateRevision); err != nil {
			return Fixture{}, err
		}
		fixture.Cases[name] = FixtureCase{WorkspaceID: ws.WorkspaceID, ItemID: item.Contract.ItemID}
	}
	body, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		return Fixture{}, err
	}
	f, err := os.OpenFile(filepath.Join(root, markerName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Fixture{}, err
	}
	if _, err = f.Write(append(body, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return Fixture{}, err
	}
	if closeErr != nil {
		return Fixture{}, closeErr
	}
	return fixture, nil
}

// Bound fixes all authority at trusted process launch. MCP arguments cannot
// select a filesystem root, workspace, item, or another case's namespace.
type Bound struct {
	mu      sync.Mutex
	root    string
	name    string
	fixture FixtureCase
	store   execution.Store
}

func Open(root, name string) (*Bound, error) {
	if name != "case-a" && name != "case-b" {
		return nil, errors.New("unknown synthetic workspace")
	}
	if err := safeRoot(root); err != nil {
		return nil, err
	}
	if err := safeTree(root); err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(root, markerName))
	if err != nil {
		return nil, errors.New("synthetic fixture marker required")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
		return nil, errors.New("invalid synthetic marker")
	}
	dec := json.NewDecoder(io.LimitReader(f, 16385))
	dec.DisallowUnknownFields()
	var fixture Fixture
	if err := dec.Decode(&fixture); err != nil {
		return nil, errors.New("invalid synthetic marker")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, errors.New("invalid synthetic marker suffix")
	}
	if fixture.SchemaVersion != 1 || fixture.Purpose != "maestro-mcp-synthetic-only" || fixture.Root != root || len(fixture.Cases) != 2 {
		return nil, errors.New("synthetic marker does not bind this root")
	}
	ids := regexp.MustCompile(`^[a-f0-9]{32}$`)
	items := regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,95}$`)
	for _, alias := range []string{"case-a", "case-b"} {
		entry, ok := fixture.Cases[alias]
		if !ok || !ids.MatchString(entry.WorkspaceID) || !items.MatchString(entry.ItemID) {
			return nil, errors.New("invalid synthetic case binding")
		}
		inspection, err := workspace.Inspect(filepath.Join(root, alias), filepath.Join(root, "state"))
		if err != nil || inspection.WorkspaceID != entry.WorkspaceID {
			return nil, errors.New("synthetic workspace binding changed")
		}
	}
	if fixture.Cases["case-a"].WorkspaceID == fixture.Cases["case-b"].WorkspaceID || fixture.Cases["case-a"].ItemID == fixture.Cases["case-b"].ItemID {
		return nil, errors.New("synthetic cases must be distinct")
	}
	b := &Bound{root: root, name: name, fixture: fixture.Cases[name], store: execution.Store{Root: filepath.Join(root, "state")}}
	if _, err := b.store.Inspect(b.fixture.WorkspaceID, b.fixture.ItemID); err != nil {
		return nil, errors.New("synthetic execution item unavailable")
	}
	return b, nil
}

func (b *Bound) Context(ctx context.Context) (ContextOutput, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.context(ctx)
}

func (b *Bound) context(ctx context.Context) (ContextOutput, error) {
	if err := b.ready(ctx); err != nil {
		return ContextOutput{}, err
	}
	next, err := b.store.Next(b.fixture.WorkspaceID, b.fixture.ItemID)
	if err != nil {
		return ContextOutput{}, errors.New("bound case context unavailable")
	}
	return ContextOutput{Workspace: b.name, ItemID: next.ItemID, State: string(next.State), StateRevision: next.StateRevision, AttemptID: next.AttemptID, Summary: next.Summary, NextStep: next.NextStep, Blocker: next.Blocker, ArtifactRefs: next.ArtifactRefs}, nil
}

func (b *Bound) Checkpoint(ctx context.Context, input CheckpointInput) (CheckpointOutput, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.ready(ctx); err != nil {
		return CheckpointOutput{}, err
	}
	item, err := b.store.Checkpoint(b.fixture.WorkspaceID, b.fixture.ItemID, execution.CheckpointInput{ExpectedRevision: input.ExpectedRevision, AttemptID: input.AttemptID, Summary: input.Summary, NextStep: input.NextStep})
	if err != nil {
		return CheckpointOutput{}, errors.New("checkpoint rejected; refresh bound case context and verify size, revision and attempt")
	}
	checkpointID := item.Checkpoint.CheckpointID
	if input.Pause {
		item, err = b.store.Pause(b.fixture.WorkspaceID, b.fixture.ItemID, item.State.StateRevision, input.AttemptID)
		if err != nil {
			return CheckpointOutput{}, errors.New("checkpoint saved but pause rejected; refresh bound case context")
		}
	}
	return CheckpointOutput{Workspace: b.name, State: string(item.State.State), StateRevision: item.State.StateRevision, AttemptID: item.State.ActiveAttemptID, CheckpointID: checkpointID}, nil
}

func (b *Bound) Resume(ctx context.Context, input ResumeInput) (ContextOutput, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.ready(ctx); err != nil {
		return ContextOutput{}, err
	}
	if _, err := b.store.Resume(b.fixture.WorkspaceID, b.fixture.ItemID, input.ExpectedRevision); err != nil {
		return ContextOutput{}, errors.New("resume rejected; refresh bound case context and require paused current revision")
	}
	return b.context(ctx)
}

func (b *Bound) ready(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := safeRoot(b.root); err != nil {
		return errors.New("synthetic root is unsafe")
	}
	if err := safeTree(b.root); err != nil {
		return errors.New("synthetic fixture contains unsafe filesystem entries")
	}
	return nil
}

func safeRoot(root string) error {
	developer, err := developerRoot()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || !strings.HasPrefix(root, developer+string(filepath.Separator)) {
		return errors.New("synthetic root must be an explicit clean absolute path physically under Developer")
	}
	for p := root; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil && !os.IsNotExist(err) {
			return errors.New("cannot inspect synthetic root")
		}
		if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return errors.New("synthetic root ancestry must contain only ordinary directories")
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	return nil
}

// This fixture scan is a spike guard, not a production hostile-concurrency
// filesystem capability. Production confinement remains separate work.
func safeTree(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return errors.New("fixture contains a filesystem alias or special file")
		}
		return nil
	})
}
