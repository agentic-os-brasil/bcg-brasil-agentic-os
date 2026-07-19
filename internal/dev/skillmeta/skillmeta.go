package skillmeta

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DScardini91/bcg-brasil-agentic-os/internal/dev/clauderouting"
)

var allowedKeys = map[string]bool{"name": true, "description": true}

type claudeSettings struct {
	MinimumVersion string                          `json:"minimumVersion"`
	Permissions    json.RawMessage                 `json:"permissions"`
	Hooks          map[string][]claudeMatcherGroup `json:"hooks"`
}

type claudeMatcherGroup struct {
	Matcher string              `json:"matcher"`
	Hooks   []claudeHookHandler `json:"hooks"`
}

type claudeHookHandler struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// ValidateDir checks every development skill package under root.
func ValidateDir(root string) error {
	children, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read development skills directory: %w", err)
	}
	var problems []error
	count := 0
	for _, child := range children {
		if !child.IsDir() {
			continue
		}
		count++
		if err := validateSkill(filepath.Join(root, child.Name()), child.Name()); err != nil {
			problems = append(problems, err)
		}
	}
	if count == 0 {
		problems = append(problems, errors.New("no development skills found"))
	}
	return errors.Join(problems...)
}

// ValidateClaudeProjections ensures Claude discovers one thin projection for every canonical development skill.
func ValidateClaudeProjections(canonicalRoot, projectionRoot string) error {
	canonical, err := skillNames(canonicalRoot)
	if err != nil {
		return fmt.Errorf("read canonical skills: %w", err)
	}
	projections, err := skillNames(projectionRoot)
	if err != nil {
		return fmt.Errorf("read Claude skill projections: %w", err)
	}
	var problems []error
	for name := range canonical {
		if !projections[name] {
			problems = append(problems, fmt.Errorf("Claude projection missing for development skill %s", name))
			continue
		}
		path := filepath.Join(projectionRoot, name, "SKILL.md")
		content, err := os.ReadFile(path)
		if err != nil {
			problems = append(problems, fmt.Errorf("read Claude projection %s: %w", name, err))
			continue
		}
		text := string(content)
		expectedPointer := "../../../dev/skills/" + name + "/SKILL.md"
		expectedBody := "\n# Canonical development skill\n\nRead and follow `" + expectedPointer + "` completely. That file is authoritative; this thin Claude projection exists only for native skill discovery.\n"
		parts := strings.SplitN(text, "\n---\n", 2)
		frontmatterValid := false
		if len(parts) == 2 {
			lines := strings.Split(parts[0], "\n")
			frontmatterValid = len(lines) == 3 && lines[0] == "---" && lines[1] == "name: "+name && strings.HasPrefix(lines[2], "description: ") && strings.TrimSpace(strings.TrimPrefix(lines[2], "description: ")) != ""
		}
		if !frontmatterValid || parts[1] != expectedBody {
			problems = append(problems, fmt.Errorf("Claude projection %s must use the exact canonical pointer template", name))
		}
	}
	for name := range projections {
		if !canonical[name] {
			problems = append(problems, fmt.Errorf("Claude projection %s has no canonical development skill", name))
		}
	}
	return errors.Join(problems...)
}

// ValidateClaudeRouting enforces Claude as the primary native development surface.
func ValidateClaudeRouting(root string) error {
	manifest, err := clauderouting.Load(root)
	if err != nil {
		return err
	}

	var problems []error
	if manifest.PrimaryRuntime != "claude" {
		problems = append(problems, fmt.Errorf("primary_runtime must be claude, got %q", manifest.PrimaryRuntime))
	}
	if manifest.CanonicalRoot != "dev/skills" {
		problems = append(problems, fmt.Errorf("canonical_root must be dev/skills, got %q", manifest.CanonicalRoot))
	}
	if manifest.ProjectionRoot != ".claude/skills" {
		problems = append(problems, fmt.Errorf("projection_root must be .claude/skills, got %q", manifest.ProjectionRoot))
	}

	canonical, err := skillNames(filepath.Join(root, filepath.FromSlash(manifest.CanonicalRoot)))
	if err != nil {
		problems = append(problems, fmt.Errorf("read routed canonical skills: %w", err))
		return errors.Join(problems...)
	}
	routed := make(map[string]bool)
	for intent, name := range manifest.Routes {
		if strings.TrimSpace(intent) == "" || strings.TrimSpace(name) == "" {
			problems = append(problems, errors.New("Claude skill routes cannot contain empty intents or skill names"))
			continue
		}
		if !canonical[name] {
			problems = append(problems, fmt.Errorf("Claude route %s references unknown skill %s", intent, name))
		}
		routed[name] = true
	}
	for name := range canonical {
		if !routed[name] {
			problems = append(problems, fmt.Errorf("canonical development skill %s has no Claude intent route", name))
		}
	}
	for _, name := range manifest.GoldenPath {
		if !canonical[name] {
			problems = append(problems, fmt.Errorf("Claude golden path references unknown skill %s", name))
		}
	}
	if len(manifest.GoldenPath) == 0 {
		problems = append(problems, errors.New("Claude golden path cannot be empty"))
	}
	seenGolden := make(map[string]bool)
	for _, name := range manifest.GoldenPath {
		if seenGolden[name] {
			problems = append(problems, fmt.Errorf("Claude golden path contains duplicate skill %s", name))
		}
		seenGolden[name] = true
	}
	if manifest.Fallback == "" || !canonical[manifest.Fallback] {
		problems = append(problems, fmt.Errorf("Claude fallback references unknown skill %s", manifest.Fallback))
	}

	orientation, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		problems = append(problems, fmt.Errorf("read CLAUDE.md: %w", err))
	} else {
		text := string(orientation)
		if !strings.Contains(text, "Claude Code is the primary development runtime") {
			problems = append(problems, errors.New("CLAUDE.md must declare Claude Code as the primary development runtime"))
		}
		if !strings.Contains(text, ".claude/skill-routing.json") {
			problems = append(problems, errors.New("CLAUDE.md must reference .claude/skill-routing.json"))
		}
		for name := range canonical {
			if !strings.Contains(text, "$"+name) {
				problems = append(problems, fmt.Errorf("CLAUDE.md must route through $%s", name))
			}
		}
	}

	settingsFile, err := os.Open(filepath.Join(root, ".claude", "settings.json"))
	if err != nil {
		problems = append(problems, fmt.Errorf("read Claude settings: %w", err))
	} else {
		defer settingsFile.Close()
		var settings claudeSettings
		decoder := json.NewDecoder(settingsFile)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&settings); err != nil {
			problems = append(problems, fmt.Errorf("decode Claude settings: %w", err))
		} else {
			if settings.MinimumVersion != "2.1.177" {
				problems = append(problems, fmt.Errorf("Claude settings minimumVersion must be 2.1.177, got %q", settings.MinimumVersion))
			}
			expected := []struct {
				event   string
				matcher string
				command string
			}{
				{"SessionStart", "", `bash "$CLAUDE_PROJECT_DIR/.claude/hooks/run-dev-hook.sh" session-start`},
				{"PreToolUse", "Bash|Edit|Write", `bash "$CLAUDE_PROJECT_DIR/.claude/hooks/run-dev-hook.sh" pre-tool`},
				{"PostToolUse", "Skill", `bash "$CLAUDE_PROJECT_DIR/.claude/hooks/run-dev-hook.sh" skill-used`},
				{"PostToolUse", "Edit|Write", `bash "$CLAUDE_PROJECT_DIR/.claude/hooks/run-dev-hook.sh" post-tool`},
			}
			for _, requirement := range expected {
				if !hasClaudeHook(settings, requirement.event, requirement.matcher, requirement.command) {
					problems = append(problems, fmt.Errorf("Claude settings missing required %s hook with matcher %q and command %q", requirement.event, requirement.matcher, requirement.command))
				}
			}
			if !hasClaudeSkillExpansionHook(settings, canonical, `bash "$CLAUDE_PROJECT_DIR/.claude/hooks/run-dev-hook.sh" prompt-expansion`) {
				problems = append(problems, errors.New("Claude settings missing UserPromptExpansion coverage for every canonical development skill"))
			}
		}
	}

	return errors.Join(problems...)
}

func hasClaudeSkillExpansionHook(settings claudeSettings, canonical map[string]bool, command string) bool {
	for _, group := range settings.Hooks["UserPromptExpansion"] {
		matched := make(map[string]bool)
		for _, name := range strings.Split(group.Matcher, "|") {
			matched[strings.TrimSpace(name)] = true
		}
		if len(matched) != len(canonical) {
			continue
		}
		complete := true
		for name := range canonical {
			if !matched[name] {
				complete = false
			}
		}
		if complete && hasClaudeHook(settings, "UserPromptExpansion", group.Matcher, command) {
			return true
		}
	}
	return false
}

func hasClaudeHook(settings claudeSettings, event, matcher, command string) bool {
	for _, group := range settings.Hooks[event] {
		if group.Matcher != matcher {
			continue
		}
		for _, hook := range group.Hooks {
			if hook.Type == "command" && hook.Command == command {
				return true
			}
		}
	}
	return false
}

func skillNames(root string) (map[string]bool, error) {
	children, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	names := make(map[string]bool)
	for _, child := range children {
		if !child.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, child.Name(), "SKILL.md")); err == nil {
			names[child.Name()] = true
		}
	}
	return names, nil
}

func validateSkill(skillDir, expectedName string) error {
	file, err := os.Open(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return fmt.Errorf("skill %s: open SKILL.md: %w", expectedName, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return fmt.Errorf("skill %s: SKILL.md must start with YAML frontmatter", expectedName)
	}
	metadata := make(map[string]string)
	closed := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			closed = true
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("skill %s: malformed frontmatter line %q", expectedName, line)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if !allowedKeys[key] {
			return fmt.Errorf("skill %s: unsupported frontmatter key %q", expectedName, key)
		}
		metadata[key] = value
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("skill %s: read SKILL.md: %w", expectedName, err)
	}
	if !closed {
		return fmt.Errorf("skill %s: unclosed YAML frontmatter", expectedName)
	}
	if metadata["name"] != expectedName {
		return fmt.Errorf("skill %s: frontmatter name is %q", expectedName, metadata["name"])
	}
	if metadata["description"] == "" || strings.Contains(metadata["description"], "TODO") {
		return fmt.Errorf("skill %s: description is empty or unfinished", expectedName)
	}
	if _, err := os.Stat(filepath.Join(skillDir, "agents", "openai.yaml")); err != nil {
		return fmt.Errorf("skill %s: missing agents/openai.yaml", expectedName)
	}
	return nil
}
