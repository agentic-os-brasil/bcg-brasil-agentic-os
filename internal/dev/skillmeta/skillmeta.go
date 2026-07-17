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
