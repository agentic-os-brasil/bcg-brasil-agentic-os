package product

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspacepolicy"
)

// workspacePath is the one current-scope owner for page and content routes. It
// admits paths, not authored metadata or conversion provenance.
func (b *Bound) workspacePath(relative string) error {
	if err := b.workspacePathAdmission(relative); err != nil {
		return err
	}
	if strings.HasPrefix(relative, "brain/") && strings.HasSuffix(relative, ".md") {
		engine, err := atlasops.OpenWorkspace(atlas.Options{DataRoot: filepath.Join(b.home, "state"), WorkspacePath: b.options.WorkspacePath, WorkspaceID: b.workspaceID})
		if err != nil {
			return err
		}
		return engine.AdmitPhysicalPage(strings.TrimPrefix(relative, "brain/"))
	}
	return nil
}

func (b *Bound) workspacePathAdmission(relative string) error {
	if artifactPath(relative) != nil {
		return errors.New("workspace_path_invalid")
	}
	parts := strings.Split(relative, "/")
	if strings.EqualFold(parts[0], "brain") {
		if parts[0] != "brain" || len(parts) < 2 {
			return errors.New("workspace_namespace_invalid")
		}
		brain := parts[1:]
		if strings.EqualFold(brain[0], "accounts") && brain[0] != "accounts" {
			return errors.New("workspace_namespace_invalid")
		}
		if brain[0] == "accounts" && len(brain) > 2 && strings.EqualFold(brain[2], "cases") && brain[2] != "cases" {
			return errors.New("workspace_namespace_invalid")
		}
		if strings.EqualFold(brain[0], "owner") || strings.EqualFold(brain[0], "memory") || len(brain) > 1 && strings.EqualFold(brain[0], "atlas") && strings.EqualFold(brain[1], "owner") {
			return errors.New("workspace_path_unavailable")
		}
	}
	if !workspacepolicy.CasePathAllowed(b.options.WorkspacePath, filepath.Join(b.options.WorkspacePath, filepath.FromSlash(relative))) {
		return errors.New("workspace_case_unavailable")
	}
	return nil
}
func (b *Bound) workspacePage(page string) error {
	if artifactPath(page) != nil || !strings.HasSuffix(page, ".md") {
		return errors.New("workspace_page_invalid")
	}
	if err := b.workspacePathAdmission("brain/" + page); err != nil {
		return err
	}
	if b.observeWorkspaceTransition != nil {
		b.observeWorkspaceTransition("admitted", page)
	}
	return nil
}
func (b *Bound) workspaceEngine(effect func() error) (*atlasops.Engine, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	return atlasops.OpenWorkspaceWithPolicy(atlas.Options{DataRoot: filepath.Join(b.home, "state"), WorkspacePath: b.options.WorkspacePath, WorkspaceID: b.workspaceID}, atlasops.WorkspacePolicy{AdmitPage: b.workspacePage, BeforeMutation: effect})
}
