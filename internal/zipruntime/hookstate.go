package zipruntime

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// Check hook effect paths before publishing any writable binding. Case and
// workspace aliases remain the guard's responsibility, outside this path set.
func validateHookState(brain string) error {
	directories := map[string]bool{}
	for _, relative := range []string{
		".maestro", ".maestro/context-state", "accounts", "craft", "daily", "development", "learnings", "memory", "owner", "people", "tasks",
		"craft/methods", "craft/style", "development/cdc", "development/project-feedback", "development/upward-feedback", "development/retros",
		"memory/recent", "memory/weekly", "memory/medium-term", "memory/lifetime", "memory/policies",
		"owner/self", "owner/operating", "owner/observations", "owner/interview", "owner/interview/drafts",
	} {
		if err := checkHookPath(filepath.Join(brain, relative), true, directories); err != nil {
			return err
		}
	}
	for _, relative := range []string{
		".maestro/context-envelope.json", ".maestro/context-packet.json", ".maestro/agent-runs.json", ".maestro/agent-runs.json.tmp",
		".maestro/day-brief.json", ".maestro/index-capability.json", ".maestro/.index-failed", ".maestro/.health-requested", ".maestro/.health-last-seen", ".maestro/.migration-notice",
		".maestro/.darwin-requested", ".maestro/.darwin-requested.tmp", ".maestro/.gamma-requested", ".maestro/.gamma-requested.tmp",
		".initialized", ".maestro-version", ".maestro-python", ".scaffold.log", ".upgrade-pending", "FIRST-RUN-FAILED.txt", "README.md",
		"owner/registry.json", "owner/identity.json", "owner/markitdown.json", "owner/onboarding.json", "owner/.eod-requested", "owner/.no-account-ack",
		"owner/self/README.md", "owner/self/owner-identity.md", "owner/self/personal-context.md", "owner/self/professional-role.md", "owner/self/communication-style.md", "owner/self/voice.md", "owner/self/preferences.md", "owner/self/motivations.md", "owner/self/quality-bar.md", "owner/self/decision-rules.md", "owner/self/working-boundaries.md",
		"owner/operating/work-state.md", "owner/observations/observations.jsonl", "owner/interview/confirmations.json",
		"craft/craft.md", "learnings/learnings.md", "development/objectives.md",
		"memory/.gitignore", "memory/.schema-version", "memory/policies/lifetime.json", "memory/.dream-requested", "memory/.dream-ack", "memory/.dream-legacy-request",
	} {
		if err := checkHookPath(filepath.Join(brain, relative), false, directories); err != nil {
			return err
		}
	}
	// Only shell-generated dynamic leaves are scanned. Daily sources, queues and
	// recovery obstructions retain the engine's no-follow, typed refusal contract.
	for _, relative := range []string{".", ".maestro", ".maestro/context-state"} {
		dir := filepath.Join(brain, relative)
		info, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			continue
		}
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			generated := strings.HasPrefix(entry.Name(), ".recovered-")
			if relative == ".maestro" {
				generated = strings.HasPrefix(entry.Name(), "context-envelope.")
			}
			if relative == ".maestro/context-state" {
				generated = strings.HasPrefix(entry.Name(), "context-inject-") && strings.HasSuffix(entry.Name(), ".marker")
			}
			if generated {
				if err := checkHookPath(filepath.Join(dir, entry.Name()), false, directories); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func checkHookPath(path string, directory bool, directories map[string]bool) error {
	if directory {
		_, err := hookDirectory(path, directories)
		return err
	}
	available, err := hookDirectory(filepath.Dir(path), directories)
	if err != nil || !available {
		return err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("nonregular hook output")
	}
	file, err := dailyOpenRegular(path)
	if err != nil {
		return err
	}
	return file.Close()
}

// Directory observations are reused only within this read-only admission call.
func hookDirectory(path string, directories map[string]bool) (bool, error) {
	if available, checked := directories[path]; checked {
		return available, nil
	}
	parent := filepath.Dir(path)
	if parent != path {
		available, err := hookDirectory(parent, directories)
		if err != nil || !available {
			directories[path] = false
			return false, err
		}
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		directories[path] = false
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("aliased hook state")
	}
	// A regular obstruction cannot redirect a write; its consumer owns recovery.
	if !info.IsDir() {
		directories[path] = false
		return false, nil
	}
	if err := scheduler.ValidateDirectory(path); err != nil {
		return false, err
	}
	directories[path] = true
	return true, nil
}
