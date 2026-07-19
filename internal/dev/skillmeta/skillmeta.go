package skillmeta

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var allowedKeys = map[string]bool{"name": true, "description": true}

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
		if !strings.Contains(text, "name: "+name) {
			problems = append(problems, fmt.Errorf("Claude projection %s has incorrect name", name))
		}
		expectedPointer := "../../../dev/skills/" + name + "/SKILL.md"
		if !strings.Contains(text, expectedPointer) {
			problems = append(problems, fmt.Errorf("Claude projection %s must point to %s", name, expectedPointer))
		}
		if len(content) > 1200 {
			problems = append(problems, fmt.Errorf("Claude projection %s is not thin (%d bytes)", name, len(content)))
		}
	}
	for name := range projections {
		if !canonical[name] {
			problems = append(problems, fmt.Errorf("Claude projection %s has no canonical development skill", name))
		}
	}
	return errors.Join(problems...)
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
