package docling

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
)

// RevalidateSource checks a previously converted selection against its exact
// digest using the producer's single-link, no-follow, same-handle reader.
// It uses canonical source limits and never changes source permissions or loads a runtime.
func (r *Runtime) RevalidateSource(ctx context.Context, source ingest.Request, workspaceID, expectedSHA256 string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return errors.New("source_revalidation_unavailable")
	}
	if !relativeName(source.SourcePath) || !filepath.IsAbs(source.WorkspacePath) || filepath.Clean(source.WorkspacePath) != source.WorkspacePath || !r.boundWorkspace(source.WorkspacePath, workspaceID) {
		return errors.New("source_scope_invalid")
	}
	source.SourcePath = filepath.Join(source.WorkspacePath, filepath.FromSlash(source.SourcePath))
	source.Policy = ingest.DefaultPolicy()
	selected, err := source.Validate()
	if err != nil {
		return errors.New("source_changed_or_unsafe")
	}
	body, opened, err := readFile(source.SourcePath, source.Policy.MaxInputBytes)
	if err != nil || !os.SameFile(selected, opened) || digest(body) != expectedSHA256 {
		return errors.New("source_changed_or_unsafe")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !r.boundWorkspace(source.WorkspacePath, workspaceID) {
		return errors.New("source_scope_invalid")
	}
	return nil
}
