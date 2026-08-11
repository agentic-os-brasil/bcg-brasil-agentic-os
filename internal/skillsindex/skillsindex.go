// Package skillsindex compiles compact navigation pointers from canonical
// managed product skills. It never copies complete skill instructions.
package skillsindex

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Catalog struct {
	SchemaVersion int     `json:"schema_version"`
	Skills        []Skill `json:"skills"`
}

type Skill struct {
	ID            string `json:"id"`
	DisplayName   string `json:"display_name"`
	Trigger       string `json:"trigger"`
	DefaultPrompt string `json:"default_prompt"`
	RelativePath  string `json:"relative_path"`
	Bundle        string `json:"-"`
}

func Build(root string) (Catalog, error) {
	children, err := os.ReadDir(root)
	if err != nil {
		return Catalog{}, err
	}
	catalog := Catalog{SchemaVersion: 1}
	for _, child := range children {
		if !child.IsDir() {
			continue
		}
		id := child.Name()
		skillPath := filepath.Join(root, id, "SKILL.md")
		body, err := os.ReadFile(skillPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return Catalog{}, err
		}
		name, _, err := frontmatter(body)
		if err != nil {
			return Catalog{}, fmt.Errorf("read %s: %w", id, err)
		}
		if name != id {
			return Catalog{}, fmt.Errorf("skill %s has frontmatter name %q", id, name)
		}
		metadata, err := os.ReadFile(filepath.Join(root, id, "agents", "openai.yaml"))
		if err != nil {
			return Catalog{}, fmt.Errorf("read runtime metadata for %s: %w", id, err)
		}
		displayName := yamlValue(metadata, "display_name")
		trigger := yamlValue(metadata, "short_description")
		prompt := yamlValue(metadata, "default_prompt")
		if displayName == "" || trigger == "" || prompt == "" {
			return Catalog{}, fmt.Errorf("skill %s lacks display_name, short_description or default_prompt", id)
		}
		catalog.Skills = append(catalog.Skills, Skill{
			ID: id, DisplayName: displayName, Trigger: trigger, DefaultPrompt: prompt,
			RelativePath: "skills/" + id + "/SKILL.md",
		})
	}
	sort.Slice(catalog.Skills, func(left, right int) bool { return catalog.Skills[left].ID < catalog.Skills[right].ID })
	return catalog, catalog.Validate()
}

func Parse(reader io.Reader) (Catalog, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var catalog Catalog
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Catalog{}, errors.New("skills catalog contains multiple JSON values")
		}
		return Catalog{}, err
	}
	return catalog, catalog.Validate()
}

func (catalog Catalog) Validate() error {
	if catalog.SchemaVersion != 1 || len(catalog.Skills) == 0 {
		return errors.New("skills catalog must use schema version 1 and contain skills")
	}
	previous := ""
	for _, skill := range catalog.Skills {
		if skill.ID == "" || skill.ID <= previous || skill.DisplayName == "" || skill.Trigger == "" || skill.DefaultPrompt == "" || skill.RelativePath != "skills/"+skill.ID+"/SKILL.md" {
			return errors.New("skills catalog contains an invalid or unsorted pointer")
		}
		previous = skill.ID
	}
	return nil
}

func RenderJSON(catalog Catalog) ([]byte, error) {
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	body, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func RenderMarkdown(catalog Catalog) ([]byte, error) {
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	var output strings.Builder
	output.WriteString("# BCGOS Skills Index\n\n")
	output.WriteString("> Generated from canonical product skills. Open a pointed SKILL.md only when the task requires it.\n\n")
	output.WriteString("| Skill | Use when | Pointer |\n|---|---|---|\n")
	for _, skill := range catalog.Skills {
		fmt.Fprintf(&output, "| %s | %s | `%s` |\n", skill.DisplayName, skill.Trigger, skill.RelativePath)
	}
	return []byte(output.String()), nil
}

func Validate(root string) error {
	catalog, err := Build(root)
	if err != nil {
		return err
	}
	expectedJSON, err := RenderJSON(catalog)
	if err != nil {
		return err
	}
	expectedMarkdown, err := RenderMarkdown(catalog)
	if err != nil {
		return err
	}
	for _, artifact := range []struct {
		name     string
		expected []byte
	}{{"catalog.json", expectedJSON}, {"INDEX.md", expectedMarkdown}} {
		actual, err := os.ReadFile(filepath.Join(root, artifact.name))
		if err != nil {
			return err
		}
		if !bytes.Equal(normalizeLineEndings(actual), artifact.expected) {
			return fmt.Errorf("skills index %s is stale; regenerate it from canonical skills", artifact.name)
		}
	}
	return nil
}

// normalizeLineEndings keeps generated-artifact validation portable across
// Git checkouts that convert managed text files to CRLF on Windows.
func normalizeLineEndings(value []byte) []byte {
	return bytes.ReplaceAll(value, []byte("\r\n"), []byte("\n"))
}

func Write(root string) error {
	catalog, err := Build(root)
	if err != nil {
		return err
	}
	jsonBody, err := RenderJSON(catalog)
	if err != nil {
		return err
	}
	markdownBody, err := RenderMarkdown(catalog)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "catalog.json"), jsonBody, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "INDEX.md"), markdownBody, 0o644)
}

func frontmatter(body []byte) (string, string, error) {
	lines := strings.Split(string(body), "\n")
	if len(lines) < 4 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", errors.New("missing YAML frontmatter")
	}
	values := map[string]string{}
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "---" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			values[strings.TrimSpace(parts[0])] = strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		}
	}
	if values["name"] == "" || values["description"] == "" {
		return "", "", errors.New("frontmatter requires name and description")
	}
	return values["name"], values["description"], nil
}

func yamlValue(body []byte, key string) string {
	prefix := key + ":"
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, prefix)), `"'`)
		}
	}
	return ""
}
