package packaging

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

type instructionSourceError string

func (e instructionSourceError) Error() string { return string(e) }
func nativeConfigurationReason(err error) string {
	var unsupported instructionSourceError
	if errors.As(err, &unsupported) {
		return string(unsupported)
	}
	return "native_configuration_unavailable"
}

// This is a conservative finite source superset, not a reimplementation of
// Code's precedence/exclude glob engine. Both instruction alternatives and all
// rules participate even when this particular prompt might not load them.
type instructionSources struct {
	files         map[string]string
	active        map[string]bool
	parsed        map[string]bool
	settingsFiles map[string]bool
	bytes, count  int
	autoDisabled  bool
}

func newInstructionSources() *instructionSources {
	return &instructionSources{files: map[string]string{}, active: map[string]bool{}, parsed: map[string]bool{}, settingsFiles: map[string]bool{}}
}
func sourceUnsupported(reason string) error {
	return instructionSourceError("native_instruction_" + reason + "_unsupported")
}

func (s *instructionSources) file(path string, instruction bool, depth int) error {
	path = filepath.Clean(path)
	if s.active[path] {
		return sourceUnsupported("import_cycle")
	}
	mode := path + ":instruction"
	if !instruction {
		mode = path + ":snapshot"
		if s.settingsFiles[path] {
			mode = path + ":settings"
		}
	}
	if s.parsed[mode] {
		return nil
	}
	if depth > 4 {
		return sourceUnsupported("import_depth")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if depth > 0 {
			return sourceUnsupported("missing_import")
		}
		s.files[path] = "missing"
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return sourceUnsupported("source_alias_or_type")
	}
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return sourceUnsupported("source_path")
	}
	body, readErr := io.ReadAll(io.LimitReader(f, (256<<10)+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || len(body) > 256<<10 {
		return sourceUnsupported("file_size")
	}
	snapshot := fmt.Sprintf("%s:%o", digest(body), info.Mode().Perm())
	if prior, exists := s.files[path]; exists && prior != "missing" {
		if prior != snapshot {
			return sourceUnsupported("source_changed")
		}
	} else {
		s.count++
		s.bytes += len(body)
	}
	if s.count > 256 || s.bytes > 2<<20 {
		return sourceUnsupported("closure_size")
	}
	s.files[path] = snapshot
	s.parsed[mode] = true
	if !instruction {
		if s.settingsFiles[path] {
			return s.settings(body)
		}
		return nil
	}
	imports, err := instructionImports(string(body))
	if err != nil {
		return err
	}
	s.active[path] = true
	defer delete(s.active, path)
	for _, imported := range imports {
		if strings.HasPrefix(imported, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return sourceUnsupported("home_import")
			}
			imported = filepath.Join(home, imported[2:])
		} else if !filepath.IsAbs(imported) {
			imported = filepath.Join(filepath.Dir(path), imported)
		}
		if err := s.file(imported, true, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (s *instructionSources) settings(body []byte) error {
	var value map[string]json.RawMessage
	if json.Unmarshal(body, &value) != nil {
		return sourceUnsupported("settings_schema")
	}
	if raw, ok := value["autoMemoryEnabled"]; ok {
		if strings.TrimSpace(string(raw)) != "false" {
			return sourceUnsupported("auto_memory")
		}
		s.autoDisabled = true
	}
	for _, key := range []string{"autoMemoryDirectory", "additionalDirectories"} {
		if raw, ok := value[key]; ok && string(raw) != "null" && string(raw) != `""` && string(raw) != "[]" {
			return sourceUnsupported("additional_sources")
		}
	}
	if raw, ok := value["permissions"]; ok {
		var permissions map[string]json.RawMessage
		if json.Unmarshal(raw, &permissions) != nil {
			return sourceUnsupported("settings_schema")
		}
		if raw, ok := permissions["additionalDirectories"]; ok && string(raw) != "[]" && string(raw) != "null" {
			return sourceUnsupported("additional_directories")
		}
	}
	if raw, ok := value["enabledPlugins"]; ok {
		var plugins map[string]bool
		if json.Unmarshal(raw, &plugins) != nil {
			return sourceUnsupported("plugins")
		}
		for _, enabled := range plugins {
			if enabled {
				return sourceUnsupported("plugins")
			}
		}
	}
	// Settings can also set startup environment. Bind these bytes but refuse
	// settings-driven alternate source controls instead of predicting precedence.
	if raw, ok := value["env"]; ok {
		var env map[string]string
		if json.Unmarshal(raw, &env) != nil {
			return sourceUnsupported("settings_schema")
		}
		for key := range env {
			if instructionEnvironmentControl(key) {
				return sourceUnsupported("settings_environment")
			}
		}
	}
	return nil
}

func instructionEnvironmentControl(key string) bool {
	switch key {
	case "CLAUDE_CONFIG_DIR", "CLAUDE_CODE_DISABLE_AUTO_MEMORY", "CLAUDE_CODE_DISABLE_CLAUDE_MDS", "CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD", "CLAUDE_CODE_AUTO_MEMORY_PATH", "CLAUDE_CODE_AUTO_MEMORY_DIR":
		return true
	}
	return false
}
func (s *instructionSources) environment() error {
	// Unknown source-expanding controls refuse by presence. Do not fingerprint
	// arbitrary CLAUDE/ANTHROPIC variables, which may contain credentials.
	for _, key := range []string{"CLAUDE_CODE_DISABLE_CLAUDE_MDS", "CLAUDE_CODE_AUTO_MEMORY_PATH", "CLAUDE_CODE_AUTO_MEMORY_DIR"} {
		if _, present := os.LookupEnv(key); present {
			return sourceUnsupported("environment_sources")
		}
	}
	// Deliberate documented instruction/memory controls only, plus HOME for
	// documented ~/ imports. Values never appear in the plan or diagnostics.
	for _, key := range []string{"HOME", "CLAUDE_CONFIG_DIR", "CLAUDE_CODE_DISABLE_AUTO_MEMORY", "CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD"} {
		value, present := os.LookupEnv(key)
		if present && value != "" && key == "CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD" {
			return sourceUnsupported("environment_sources")
		}
		s.files["environment:"+key] = fmt.Sprint(present) + ":" + digest([]byte(value))
	}
	return nil
}

func (s *instructionSources) root(root string, activeSettings ...string) error {
	_, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		s.files["root:"+root] = "missing"
		return nil
	}
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return sourceUnsupported("root")
	}
	if err := scheduler.ValidateDirectory(resolved); err != nil {
		return sourceUnsupported("root")
	}
	s.files["root:"+root] = resolved
	for _, name := range activeSettings {
		s.settingsFiles[filepath.Join(resolved, name)] = true
	}
	names := []string{"CLAUDE.md", "CLAUDE.local.md", "AGENTS.md"}
	for _, name := range names {
		if err := s.file(filepath.Join(resolved, name), true, 0); err != nil {
			return err
		}
	}
	for _, name := range []string{"settings.json", "settings.local.json"} {
		if err := s.file(filepath.Join(resolved, name), false, 0); err != nil {
			return err
		}
	}
	return s.rules(filepath.Join(resolved, "rules"), 0)
}

func (s *instructionSources) rules(root string, depth int) error {
	if depth > 32 {
		return sourceUnsupported("rules_depth")
	}
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		s.files["rules:"+root] = "missing"
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return sourceUnsupported("rule_alias_or_type")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	if len(entries) > 256 || len(s.files) > 2048 {
		return sourceUnsupported("rules_membership")
	}
	members := []string{}
	for _, entry := range entries {
		members = append(members, entry.Name()+":"+entry.Type().String())
		path := filepath.Join(root, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			return sourceUnsupported("rule_alias_or_type")
		}
		if entry.IsDir() {
			if err := s.rules(path, depth+1); err != nil {
				return err
			}
		} else if strings.HasSuffix(entry.Name(), ".md") {
			if err := s.file(path, true, 0); err != nil {
				return err
			}
		} else if !entry.Type().IsRegular() {
			return sourceUnsupported("rule_alias_or_type")
		}
	}
	sort.Strings(members)
	body, _ := json.Marshal(members)
	s.files["rules:"+root] = digest(body)
	return nil
}

func (s *instructionSources) ancestry(directory string, settingsPolicy bool) error {
	projectRoot := ""
	if settingsPolicy {
		var err error
		projectRoot, err = s.projectLocalRoot(directory)
		if err != nil {
			return err
		}
	}
	for root, depth := filepath.Clean(directory), 0; ; root, depth = filepath.Dir(root), depth+1 {
		if depth > 64 {
			return sourceUnsupported("ancestry")
		}
		for _, name := range []string{"CLAUDE.md", "CLAUDE.local.md", "AGENTS.md"} {
			if err := s.file(filepath.Join(root, name), true, 0); err != nil {
				return err
			}
		}
		activeSettings := []string{}
		if settingsPolicy && root == directory {
			activeSettings = []string{"settings.json", "settings.local.json"}
		}
		if err := s.root(filepath.Join(root, ".claude"), activeSettings...); err != nil {
			return err
		}
		// Shared settings stay at cwd. The root local file also applies in a
		// normal Git checkout, alongside legacy cwd-local settings. A worktree's
		// main-checkout resolution is deliberately unsupported, never guessed.
		if settingsPolicy && root == projectRoot && root != directory {
			configurationRoot, exists := s.files["root:"+filepath.Join(root, ".claude")]
			if !exists {
				return sourceUnsupported("project_local_root")
			}
			if configurationRoot != "missing" {
				path := filepath.Join(configurationRoot, "settings.local.json")
				s.settingsFiles[path] = true
				if err := s.file(path, false, 0); err != nil {
					return err
				}
			}
		}
		if filepath.Dir(root) == root {
			break
		}
	}
	return nil
}

func (s *instructionSources) projectLocalRoot(directory string) (string, error) {
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_CEILING_DIRECTORIES", "GIT_DISCOVERY_ACROSS_FILESYSTEM", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS"} {
		if value, present := os.LookupEnv(key); present && value != "" {
			return "", sourceUnsupported("project_environment")
		}
	}
	for root, depth := filepath.Clean(directory), 0; ; root, depth = filepath.Dir(root), depth+1 {
		if depth > 64 {
			return "", sourceUnsupported("project_ancestry")
		}
		marker := filepath.Join(root, ".git")
		info, err := os.Lstat(marker)
		if err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return "", sourceUnsupported("worktree_or_git_alias")
			}
			if err := scheduler.ValidateDirectory(marker); err != nil {
				return "", sourceUnsupported("git_root")
			}
			// Unusual or incomplete repository layouts cannot prove which local
			// settings root Code would discover. No Git command or index is used.
			for _, name := range []string{"HEAD", "config"} {
				if err := s.file(filepath.Join(marker, name), false, 0); err != nil {
					return "", err
				}
				if s.files[filepath.Join(marker, name)] == "missing" {
					return "", sourceUnsupported("git_root")
				}
			}
			if err := scheduler.ValidateDirectory(filepath.Join(marker, "objects")); err != nil {
				return "", sourceUnsupported("git_root")
			}
			config, err := scheduler.OpenRegularFile(filepath.Join(marker, "config"))
			if err != nil {
				return "", sourceUnsupported("git_root")
			}
			body, readErr := io.ReadAll(io.LimitReader(config, (256<<10)+1))
			closeErr := config.Close()
			if readErr != nil || closeErr != nil || len(body) > 256<<10 {
				return "", sourceUnsupported("git_root")
			}
			lower := strings.ToLower(string(body))
			if strings.Contains(lower, "worktree") || strings.Contains(lower, "[include") || strings.Contains(lower, "[extensions") {
				return "", sourceUnsupported("git_configuration")
			}

			for _, line := range strings.Split(lower, "\n") {
				key, value, hasValue := strings.Cut(strings.TrimSpace(line), "=")
				if strings.TrimSpace(key) == "bare" && (!hasValue || strings.TrimSpace(value) != "false") {
					return "", sourceUnsupported("git_configuration")
				}
			}

			s.files["project-local-root"] = root
			home, _ := os.UserHomeDir()
			if root == home {
				return directory, nil
			}
			return root, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		s.files["git-marker:"+marker] = "missing"
		if filepath.Dir(root) == root {
			return directory, nil
		}
	}
}

func instructionConfiguration(directory string, paths ...string) (string, error) {
	if !filepath.IsAbs(directory) {
		return "", sourceUnsupported("configuration_location")
	}
	s := newInstructionSources()
	if err := s.environment(); err != nil {
		return "", err
	}
	if err := s.root(directory, "settings.json"); err != nil {
		return "", err
	}
	for index, path := range paths {
		if err := s.ancestry(path, index == 0); err != nil {
			return "", err
		}
	}
	managed := "/etc/claude-code"
	if runtime.GOOS == "darwin" {
		managed = "/Library/Application Support/ClaudeCode"
	}
	if err := s.root(managed); err != nil {
		return "", err
	}
	managedRoot := managed
	if resolved := s.files["root:"+managed]; resolved != "" && resolved != "missing" {
		managedRoot = resolved
	}
	managedFile := filepath.Join(managedRoot, "managed-settings.json")
	s.settingsFiles[managedFile] = true
	if err := s.file(managedFile, false, 0); err != nil {
		return "", err
	}
	if info, err := os.Lstat(filepath.Join(managed, "managed-settings.d")); err == nil {
		if !info.IsDir() {
			return "", sourceUnsupported("managed_sources")
		}
		entries, err := os.ReadDir(filepath.Join(managed, "managed-settings.d"))
		if err != nil {
			return "", err
		}
		if len(entries) > 0 {
			return "", sourceUnsupported("managed_sources")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if !s.autoDisabled {
		return "", sourceUnsupported("auto_memory")
	}
	body, err := json.Marshal(s.files)
	return digest(body), err
}

// Supported imports: relative/absolute/~/ paths, escaped spaces, inline code
// spans and fenced Markdown blocks. Ambiguous syntax refuses; it never drops a
// possible source silently. Depth/cycle/bytes/aliases are checked by the reader.
func instructionImports(body string) ([]string, error) {
	if !utf8.ValidString(body) {
		return nil, sourceUnsupported("encoding")
	}
	imports := []string{}
	fence := ""
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			marker := trimmed[:3]
			if fence == "" {
				fence = marker
			} else if fence == marker {
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		for i := 0; i < len(line); {
			if line[i] == '`' {
				n := 1
				for i+n < len(line) && line[i+n] == '`' {
					n++
				}
				end := strings.Index(line[i+n:], strings.Repeat("`", n))
				if end < 0 {
					return nil, sourceUnsupported("import_syntax")
				}
				i += n + end + n
				continue
			}
			if line[i] != '@' {
				i++
				continue
			}
			if i > 0 && !unicode.IsSpace(rune(line[i-1])) {
				return nil, sourceUnsupported("import_syntax")
			}
			i++
			var path strings.Builder
			for i < len(line) && !unicode.IsSpace(rune(line[i])) {
				if line[i] == '\\' {
					if i+1 >= len(line) || line[i+1] != ' ' {
						return nil, sourceUnsupported("import_syntax")
					}
					path.WriteByte(' ')
					i += 2
					continue
				}
				if strings.ContainsRune("\"'`$*?<>|{}[]()", rune(line[i])) {
					return nil, sourceUnsupported("import_syntax")
				}
				path.WriteByte(line[i])
				i++
			}
			value := path.String()
			if value == "" || strings.HasPrefix(value, "~") && !strings.HasPrefix(value, "~/") || strings.HasSuffix(value, ",") || strings.HasSuffix(value, ";") {
				return nil, sourceUnsupported("import_syntax")
			}
			imports = append(imports, value)
		}
	}
	if fence != "" {
		return nil, sourceUnsupported("import_syntax")
	}
	return imports, nil
}
