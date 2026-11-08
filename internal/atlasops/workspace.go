package atlasops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

// workspaceScope is private control metadata, never a model-selected root.
type workspaceScope struct {
	DataRoot      string `json:"data_root"`
	WorkspacePath string `json:"workspace_path"`
	WorkspaceID   string `json:"workspace_id"`
}

// WorkspacePolicy is supplied by the admitted host, never by a page/tool
// argument. Metadata validation remains a separate authoring concern.
type WorkspacePolicy struct {
	ExpectedLifecycleRevision *uint64
	AdmitPage                 func(string) error
	BeforeMutation            func() error
}

// OpenWorkspace opens only the exact registered workspace brain without pages
// or a host policy, preserving intentional legacy kernel callers.
func OpenWorkspace(options atlas.Options) (*Engine, error) {
	return OpenWorkspaceWithPolicy(options, WorkspacePolicy{})
}

// OpenWorkspaceWithPolicy binds current page/source authority to the owning
// reader and serialized mutation boundary. The original constructor remains
// unchanged for intentional legacy kernel/owner callers.
func OpenWorkspaceWithPolicy(options atlas.Options, policy WorkspacePolicy) (*Engine, error) {
	if strings.TrimSpace(options.DataRoot) == "" || strings.TrimSpace(options.WorkspacePath) == "" || strings.TrimSpace(options.WorkspaceID) == "" {
		return nil, errors.New("workspace atlas requires data root, registered workspace path and workspace id")
	}
	dataRoot, err := scheduler.CanonicalPrivatePath(options.DataRoot)
	if err != nil {
		return nil, err
	}
	workspacePath, err := scheduler.CanonicalPrivatePath(options.WorkspacePath)
	if err != nil {
		return nil, err
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	engine := &Engine{root: filepath.Join(workspacePath, "brain"), dataRoot: dataRoot, now: now, workspacePolicy: policy,
		workspace: &workspaceScope{DataRoot: dataRoot, WorkspacePath: workspacePath, WorkspaceID: options.WorkspaceID}}
	if err := engine.validateBinding(); err != nil {
		return nil, err
	}
	for _, path := range []string{dataRoot, workspacePath, engine.root} {
		info, err := boundAtlasDirectory(path)
		if err != nil {
			return nil, err
		}
		engine.boundDirectories = append(engine.boundDirectories, info)
	}
	return engine, nil
}

func (engine *Engine) validateBinding() error {
	if engine.workspace == nil {
		return scheduler.ValidatePrivateDirectory(engine.root)
	}
	scope := engine.workspace
	for index, path := range []string{scope.DataRoot, scope.WorkspacePath, engine.root} {
		if err := scheduler.ValidateDirectory(path); err != nil {
			return err
		}
		if len(engine.boundDirectories) != 0 {
			info, err := boundAtlasDirectory(path)
			if err != nil || !os.SameFile(engine.boundDirectories[index], info) {
				return errors.New("workspace atlas directory binding changed")
			}
		}
	}
	registered, err := workspace.ResolveReference(scope.WorkspaceID, scope.DataRoot)
	if err != nil {
		return err
	}
	if registered != scope.WorkspacePath {
		return errors.New("workspace atlas requires the exact registered workspace path and id")
	}
	// ResolveReference checks the registry; these no-follow reads also refuse
	// an aliased manifest or an incomplete/replaced brain.
	if _, err := readAtlasFile(filepath.Join(scope.WorkspacePath, ".bcgos", "workspace.json"), 64<<10, true); err != nil {
		return err
	}
	if _, err := readAtlasFile(filepath.Join(engine.root, "README.md"), maximumPageBytes, true); err != nil {
		return err
	}
	return nil
}

func (engine *Engine) validateScope(scope *workspaceScope) error {
	if scope == nil && engine.workspace == nil {
		return nil
	}
	if scope == nil || engine.workspace == nil || *scope != *engine.workspace {
		return errors.New("atlas control belongs to another workspace or owner scope")
	}
	return nil
}

func (engine *Engine) pointer(page string) string {
	if engine.workspace != nil {
		return "bcgos://atlas/workspace/" + engine.workspace.WorkspaceID + "/" + page
	}
	return "bcgos://atlas/owner/" + page
}

// WorkspaceMutationReady reports the owning platform gate without touching
// state. Binding admission and actual mutation checks still run separately.
func WorkspaceMutationReady() bool { return workspaceMutationAvailable() == nil }

func (engine *Engine) admitWorkspacePage(page string) error {
	if engine.workspacePolicy.AdmitPage != nil {
		return engine.workspacePolicy.AdmitPage(page)
	}
	return nil
}
