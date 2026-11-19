package packaging

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const ForegroundManagedPolicyBoundary = "Remote or OS-delivered managed policy is UNATTESTED and may affect startup, connections, context or effects. The native probe observes only the supported local closure; it does not attest enterprise device policy or production admission."
const ForegroundContextBoundary = "The dedicated native profile and working directory use their reviewed effective local instruction alternatives, ancestry, imported rules and settings. That startup context may be loaded and sent to the native provider in BOTH the synthetic qualification session and the selected role session. The qualification input contains only a fresh synthetic challenge; task/context/SELF are withheld until the local native qualification gate succeeds."

func VerifyForegroundConfiguration(ctx context.Context, r product.ForegroundRuntime) error {
	managed, err := nativeManagedDirectory()
	if err != nil {
		return err
	}
	return verifyForegroundConfigurationAt(ctx, r, managed)
}
func verifyForegroundConfigurationAt(ctx context.Context, r product.ForegroundRuntime, managed string) error {
	fresh, err := observeWeeklyConfigurationAt(ctx, r.ProfileDirectory, r.WorkingDirectory, managed)
	if err != nil {
		return err
	}
	if product.ForegroundDigest(fresh) != product.ForegroundDigest(r.Configuration) {
		return errors.New("foreground_configuration_sources_changed")
	}
	return nil
}

// ObserveForegroundLocalClosure reuses the original finite source reader with
// the child's explicit HOME/environment rather than the coordinator's profile.
func ObserveForegroundLocalClosure(ctx context.Context, r product.ForegroundRuntime) (string, error) {
	managed, err := nativeManagedDirectory()
	if err != nil {
		return "", err
	}
	return observeForegroundLocalClosureAt(ctx, r, managed)
}
func observeForegroundLocalClosureAt(ctx context.Context, r product.ForegroundRuntime, managed string) (string, error) {
	if ctx == nil || ctx.Err() != nil || maintenance.ValidateWeeklyModelEnvironment(r.Environment) != nil {
		return "", errors.New("foreground_local_closure_unavailable")
	}
	if err := verifyForegroundConfigurationAt(ctx, r, managed); err != nil {
		return "", err
	}
	env := map[string]string{}
	for _, entry := range r.Environment {
		key, value, _ := strings.Cut(entry, "=")
		env[key] = value
	}
	home := env["HOME"]
	if !filepath.IsAbs(home) || filepath.Clean(home) != home || env["CLAUDE_CONFIG_DIR"] != r.ProfileDirectory {
		return "", errors.New("foreground_explicit_profile_required")
	}
	s := newInstructionSources()
	s.home = home
	s.lookupEnv = func(key string) (string, bool) { v, ok := env[key]; return v, ok }
	if err := s.environment(); err != nil {
		return "", err
	}
	if err := s.root(r.ProfileDirectory, map[string]nativeSettingsScope{"settings.json": nativeSettingsUser}); err != nil {
		return "", err
	}
	if err := s.ancestry(r.WorkingDirectory, true); err != nil {
		return "", err
	}
	if err := s.root(managed, nil); err != nil {
		return "", err
	}
	managedRoot := managed
	if resolved := s.files["root:"+managed]; resolved != "" && resolved != "missing" {
		managedRoot = resolved
	}
	settings := filepath.Join(managedRoot, "managed-settings.json")
	s.settingsFiles[settings] = nativeSettingsManaged
	if err := s.file(settings, false, 0); err != nil {
		return "", err
	}
	if !s.autoDisabled || !s.syncDisabled || len(s.plugins) != 0 {
		return "", sourceUnsupported("foreground_customization")
	}
	if _, err := s.managedPluginConfiguration(r.ProfileDirectory, Options{Binding: product.Options{UserHome: home, WorkspacePath: r.WorkingDirectory}}); err != nil {
		return "", err
	}
	// The canonical configuration observer refuses populated/unsafe drop-ins.
	// Preserve absent-versus-empty membership in the reviewed local closure too.
	dropins := filepath.Join(managed, "managed-settings.d")
	d, err := scheduler.OpenDirectory(dropins)
	if errors.Is(err, os.ErrNotExist) {
		s.files["foreground-managed-dropins:"+dropins] = "missing"
	} else if err != nil {
		return "", err
	} else {
		entries, readErr := d.ReadDir(1)
		closeErr := d.Close()
		if (readErr != nil && readErr != io.EOF) || closeErr != nil || len(entries) != 0 {
			return "", sourceUnsupported("managed_sources")
		}
		s.files["foreground-managed-dropins:"+dropins] = "empty"
	}
	body, err := json.Marshal(struct {
		Files         map[string]string `json:"files"`
		ManagedPolicy string            `json:"managed_policy"`
		Context       string            `json:"context"`
	}{s.files, ForegroundManagedPolicyBoundary, ForegroundContextBoundary})
	if err != nil {
		return "", err
	}
	return digest(body), ctx.Err()
}
func VerifyForegroundRuntimeSources(ctx context.Context, r product.ForegroundRuntime) error {
	if r.LocalClosureSHA256 == "" || r.ManagedPolicyBoundary != ForegroundManagedPolicyBoundary || r.ContextBoundary != ForegroundContextBoundary {
		return errors.New("foreground_local_qualification_boundary_required")
	}
	actual, err := ObserveForegroundLocalClosure(ctx, r)
	if err != nil {
		return err
	}
	if actual != r.LocalClosureSHA256 {
		return errors.New("foreground_local_closure_changed")
	}
	return nil
}
