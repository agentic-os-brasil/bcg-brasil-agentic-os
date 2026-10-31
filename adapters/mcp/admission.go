package product

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// Preflight validates selected paths without creating state or following links.
// Fresh initialization and existing binding admission share these constraints.
func Preflight(o Options) error { _, err := resolveBinding(o); return err }
func resolveBinding(o Options) (datahome.Result, error) {
	h, err := datahome.Resolve(datahome.Options{ProductRoot: o.ProductRoot, UserHome: o.UserHome, LookupEnv: o.LookupEnv})
	if err != nil {
		return h, errors.New("home_unavailable")
	}
	path := o.WorkspacePath
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n") {
		return h, errors.New("workspace_unavailable")
	}
	for _, other := range []string{h.Home, o.ProductRoot} {
		if overlaps(other, path) {
			return h, errors.New("binding_overlap")
		}
	}
	// Validate the nearest existing ancestor with the canonical no-follow walk.
	for {
		err = scheduler.ValidateDirectory(path)
		if err == nil {
			return h, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return h, errors.New("workspace_unavailable")
		}
		parent := filepath.Dir(path)
		if parent == path {
			return h, errors.New("workspace_unavailable")
		}
		path = parent
	}
}

type artifactMetadata struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

func artifactPath(path string) error {
	if !utf8.ValidString(path) || len(path) < 1 || len(path) > 256 || filepath.IsAbs(path) || strings.ContainsAny(path, "\\:\x00\r\n") || filepath.ToSlash(filepath.Clean(path)) != path {
		return errors.New("artifact_unavailable")
	}
	for _, part := range strings.Split(path, "/") {
		if strings.HasPrefix(part, ".") {
			return errors.New("artifact_unavailable")
		}
	}
	return nil
}
func admittedArtifacts(contract execution.Contract) ([]artifactMetadata, error) {
	if len(contract.Criteria) < 1 || len(contract.Criteria) > 8 || len(contract.AllowedRefs) != len(contract.Criteria) {
		return nil, errors.New("contract_unavailable")
	}
	allowed := map[string]bool{}
	const prefix = "bcgos://workspace/"
	for _, ref := range contract.AllowedRefs {
		if !strings.HasPrefix(ref, prefix) || artifactPath(strings.TrimPrefix(ref, prefix)) != nil || allowed[ref] {
			return nil, errors.New("artifact_unavailable")
		}
		allowed[ref] = true
	}
	var out []artifactMetadata
	for _, criterion := range contract.Criteria {
		if criterion.Type != execution.CriterionArtifactSnapshot || len(criterion.ID) < 1 || len(criterion.ID) > 96 || !allowed[criterion.TargetRef] {
			return nil, errors.New("criterion_unavailable")
		}
		out = append(out, artifactMetadata{ID: criterion.ID, Path: strings.TrimPrefix(criterion.TargetRef, prefix)})
	}
	body, err := json.Marshal(out)
	if err != nil || len(body) > 4096 {
		return nil, errors.New("contract_unavailable")
	}
	return out, nil
}
