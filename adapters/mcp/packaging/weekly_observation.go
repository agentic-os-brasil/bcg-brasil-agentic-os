package packaging

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// WeeklyRuntimeObservation is measured metadata, never a native qualification,
// accepted registration or execution grant.
type WeeklyRuntimeObservation struct {
	Executable       string                                  `json:"executable"`
	ExecutableSHA256 string                                  `json:"executable_sha256"`
	RuntimeVersion   string                                  `json:"runtime_version"`
	Declaration      processidentity.WeeklyNativeDeclaration `json:"declaration"`
}

var ErrWeeklyObservationUnavailable = errors.New("weekly signed native runtime observation unavailable")

// ObserveWeeklyRuntime inspects the selected image before executing only its
// supported --version command. Signature metadata is collected from the OS and
// then verified again by the original retained-image runtime boundary. It does
// not authenticate, invoke a model, inspect credentials or qualify a job.
func ObserveWeeklyRuntime(ctx context.Context, executable string, effectiveEnv []string, directory string) (out WeeklyRuntimeObservation, err error) {
	if ctx == nil || !filepath.IsAbs(executable) || filepath.Clean(executable) != executable || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || strings.ContainsAny(executable+directory, "\x00\r\n") || !weeklyObservationEnvironment(effectiveEnv) {
		return out, ErrWeeklyObservationUnavailable
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	sha, target, err := InspectNativeExecutable(executable)
	if err != nil || target.OS != runtime.GOOS || target.Arch != runtime.GOARCH {
		return out, ErrWeeklyObservationUnavailable
	}
	declaration, err := observeWeeklyNativeDeclaration(bounded, executable, target)
	if err != nil {
		return out, err
	}
	identity, err := InspectWeeklyRuntime(bounded, executable, sha, declaration)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, identity.Close()) }()
	body, err := runWeeklyModelProcess(bounded, weeklyModelProcess{Path: executable, Args: []string{"--version"}, Directory: directory, Environment: effectiveEnv, MaxOutput: 4096, MaxStderr: 4096})
	if err != nil {
		return out, ErrWeeklyObservationUnavailable
	}
	version := parseHostVersion(body)
	if version == "" || !lifecycle.MeetsMinimum(version, EnforcementMinimumVersion) {
		return out, ErrWeeklyObservationUnavailable
	}
	if err := identity.VerifyCurrent(bounded); err != nil {
		return out, err
	}
	return WeeklyRuntimeObservation{executable, sha, version, declaration}, nil
}

// Reconstruct the original effective environment instead of inheriting a
// launcher/profile or accepting source-expanding runtime controls.
func weeklyObservationEnvironment(env []string) bool {
	if len(env) > 64 {
		return false
	}
	var base []string
	tokens := 0
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			return false
		}
		switch key {
		case "CLAUDE_CODE_MAX_OUTPUT_TOKENS":
			if tokens != 0 {
				return false
			}
			tokens, _ = strconv.Atoi(value)
		case "CLAUDE_CODE_MAX_RETRIES", "CLAUDE_CODE_MAX_TURNS", "CLAUDE_CODE_RETRY_WATCHDOG", "CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "CLAUDE_CODE_DISABLE_OFFICIAL_MARKETPLACE_AUTOINSTALL", "ENABLE_CLAUDEAI_MCP_SERVERS", "DISABLE_AUTO_COMPACT":
		default:
			base = append(base, entry)
		}
	}
	expected, err := weeklyCLIEnvironment(base, tokens)
	return err == nil && slices.Equal(expected, env)
}

// ObserveWeeklyConfiguration measures the original settings scopes for this
// explicit purpose profile. It never opens credential files. As in the original
// WeeklyConfiguration contract, this does not attest every vendor source;
// native qualification remains a separate mandatory operation.
func ObserveWeeklyConfiguration(ctx context.Context, profile, directory string) (maintenance.WeeklyConfiguration, error) {
	managed, err := nativeManagedDirectory()
	if err != nil {
		return nil, err
	}
	return observeWeeklyConfigurationAt(ctx, profile, directory, managed)
}
func observeWeeklyConfigurationAt(ctx context.Context, profile, directory, managed string) (maintenance.WeeklyConfiguration, error) {
	if ctx == nil || !filepath.IsAbs(profile) || filepath.Clean(profile) != profile || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		return nil, errWeeklyConfiguration
	}
	if err := scheduler.ValidatePrivateDirectory(profile); err != nil {
		return nil, err
	}
	if err := scheduler.ValidatePrivateDirectory(directory); err != nil {
		return nil, err
	}
	roles := map[string]string{}
	add := func(path, role string) { roles[filepath.Clean(path)] = role }
	add(filepath.Join(profile, "settings.json"), "settings")
	add(filepath.Join(profile, "settings.local.json"), "settings")
	add(filepath.Join(directory, ".claude", "settings.json"), "settings")
	add(filepath.Join(directory, ".claude", "settings.local.json"), "settings")
	sources := newInstructionSources()
	projectRoot, err := sources.projectLocalRoot(directory)
	if err != nil {
		return nil, err
	}
	if projectRoot != "" && projectRoot != directory {
		add(filepath.Join(projectRoot, ".claude", "settings.local.json"), "settings")
	}
	add(filepath.Join(managed, "managed-settings.json"), "managed_policy")
	// The original native configuration inspector refuses populated drop-in
	// policy directories. Preserve that support boundary instead of inventing
	// completeness for an unqualified policy source.
	d, err := scheduler.OpenDirectory(filepath.Join(managed, "managed-settings.d"))
	if err == nil {
		entries, readErr := d.ReadDir(1)
		closeErr := d.Close()
		if readErr != nil && readErr != io.EOF || closeErr != nil || len(entries) != 0 {
			return nil, errWeeklyConfiguration
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	paths := make([]string, 0, len(roles))
	for path := range roles {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	configuration := make(maintenance.WeeklyConfiguration, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entry := maintenance.WeeklyConfigurationEntry{Path: path, Role: roles[path]}
		f, err := scheduler.OpenRegularFile(path)
		if errors.Is(err, os.ErrNotExist) {
			entry.Absent = true
			configuration = append(configuration, entry)
			continue
		}
		if err != nil {
			return nil, err
		}
		info, statErr := f.Stat()
		if statErr != nil || info.Size() > 64<<10 || scheduler.ValidateSingleLink(f) != nil {
			f.Close()
			return nil, errWeeklyConfiguration
		}
		body, readErr := io.ReadAll(io.LimitReader(f, (64<<10)+1))
		after, afterErr := f.Stat()
		closeErr := f.Close()
		if readErr != nil || afterErr != nil || closeErr != nil || len(body) > 64<<10 || !sameHostMetadata(info, after) {
			return nil, errWeeklyConfiguration
		}
		h := sha256.Sum256(body)
		entry.SHA256 = hex.EncodeToString(h[:])
		configuration = append(configuration, entry)
	}
	if err := VerifyWeeklyConfiguration(ctx, configuration); err != nil {
		return nil, err
	}
	return configuration, nil
}
