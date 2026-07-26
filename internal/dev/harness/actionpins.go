package harness

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	fullCommitSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	actionSegment = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	localAction   = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
	dockerImage   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]*$`)
	dockerDigest  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// validateActionPins enforces immutable third-party code references in every
// GitHub workflow and composite action. It parses YAML nodes instead of
// scanning source text so flow mappings, quoted keys and nested composite
// steps cannot bypass the gate.
func validateActionPins(root string) error {
	files, err := actionYAMLFiles(root)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := validateActionYAML(file); err != nil {
			relative, relativeErr := filepath.Rel(root, file)
			if relativeErr != nil {
				relative = file
			}
			return fmt.Errorf("%s: %w", filepath.ToSlash(relative), err)
		}
	}
	return nil
}

func actionYAMLFiles(root string) ([]string, error) {
	var files []string
	roots := []struct {
		directory string
		include   func(string) bool
	}{
		{
			directory: filepath.Join(root, ".github", "workflows"),
			include: func(name string) bool {
				extension := strings.ToLower(filepath.Ext(name))
				return extension == ".yml" || extension == ".yaml"
			},
		},
		{
			directory: filepath.Join(root, ".github", "actions"),
			include: func(name string) bool {
				lower := strings.ToLower(name)
				return lower == "action.yml" || lower == "action.yaml"
			},
		},
	}
	for _, current := range roots {
		err := filepath.WalkDir(current.directory, func(file string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				if errors.Is(walkErr, os.ErrNotExist) && file == current.directory {
					return filepath.SkipDir
				}
				return walkErr
			}
			if entry.IsDir() || !current.include(entry.Name()) {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("%s must not be a symlink", file)
			}
			if !entry.Type().IsRegular() {
				return fmt.Errorf("%s must be a regular file", file)
			}
			files = append(files, file)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("enumerate GitHub Actions YAML: %w", err)
		}
	}
	sort.Strings(files)
	return files, nil
}

func validateActionYAML(file string) error {
	input, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("open YAML: %w", err)
	}
	defer input.Close()

	decoder := yaml.NewDecoder(input)
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("parse YAML: empty document")
		}
		return fmt.Errorf("parse YAML: %w", err)
	}
	if len(document.Content) == 0 {
		return fmt.Errorf("parse YAML: empty document")
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return fmt.Errorf("parse YAML: %w", err)
		}
		return fmt.Errorf("parse YAML: multiple documents are not allowed")
	}
	return validateActionNode(document.Content[0], map[*yaml.Node]bool{})
}

func validateActionNode(node *yaml.Node, ancestors map[*yaml.Node]bool) error {
	if node == nil {
		return nil
	}
	if ancestors[node] {
		return nodeError(node, "cyclic YAML aliases are not allowed")
	}
	ancestors[node] = true
	defer delete(ancestors, node)

	switch node.Kind {
	case yaml.MappingNode:
		if len(node.Content)%2 != 0 {
			return nodeError(node, "malformed YAML mapping")
		}
		seen := make(map[string]bool, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			value := node.Content[index+1]
			if key.Kind != yaml.ScalarNode {
				return nodeError(key, "complex YAML mapping keys are not allowed")
			}
			if seen[key.Value] {
				return nodeError(key, "duplicate YAML mapping key %q", key.Value)
			}
			seen[key.Value] = true
			if key.Value == "uses" {
				if err := validateUsesValue(value); err != nil {
					return err
				}
			}
			if err := validateActionNode(value, ancestors); err != nil {
				return err
			}
		}
	case yaml.SequenceNode, yaml.DocumentNode:
		for _, child := range node.Content {
			if err := validateActionNode(child, ancestors); err != nil {
				return err
			}
		}
	case yaml.AliasNode:
		if node.Alias == nil {
			return nodeError(node, "malformed YAML alias")
		}
		return validateActionNode(node.Alias, ancestors)
	case yaml.ScalarNode:
		return nil
	default:
		return nodeError(node, "unsupported YAML node")
	}
	return nil
}

func validateUsesValue(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode || (node.Tag != "!!str" && node.Tag != "") {
		return nodeError(node, "uses must be a string scalar")
	}
	if node.Style == yaml.LiteralStyle || node.Style == yaml.FoldedStyle {
		return nodeError(node, "uses must be a plain or quoted scalar, not a block scalar")
	}
	reference := node.Value
	if strings.TrimSpace(reference) != reference || strings.ContainsAny(reference, "\r\n\t ") {
		return nodeError(node, "uses contains whitespace")
	}
	switch {
	case strings.HasPrefix(reference, "./"):
		if err := validateLocalAction(reference); err != nil {
			return nodeError(node, "%v", err)
		}
		return nil
	case strings.HasPrefix(reference, "docker://"):
		if err := validateDockerAction(reference); err != nil {
			return nodeError(node, "%v", err)
		}
		return nil
	default:
		if err := validateExternalAction(reference); err != nil {
			return nodeError(node, "%v", err)
		}
		return nil
	}
}

func validateLocalAction(reference string) error {
	remainder := strings.TrimPrefix(reference, "./")
	if remainder == "" ||
		strings.ContainsAny(remainder, `\@?#`) ||
		strings.HasPrefix(remainder, "/") ||
		!localAction.MatchString(remainder) ||
		path.Clean(remainder) != remainder ||
		remainder == "." ||
		strings.HasPrefix(remainder, "../") {
		return fmt.Errorf("local action must use a safe repository-relative path beginning with ./")
	}
	return nil
}

func validateDockerAction(reference string) error {
	target := strings.TrimPrefix(reference, "docker://")
	image, digest, found := strings.Cut(target, "@sha256:")
	if !found || image == "" || !dockerImage.MatchString(image) || !dockerDigest.MatchString(digest) {
		return fmt.Errorf("docker action must use docker://IMAGE@sha256: followed by a lowercase 64-hex sha256 digest")
	}
	return nil
}

func validateExternalAction(reference string) error {
	action, revision, found := strings.Cut(reference, "@")
	if !found || strings.Contains(revision, "@") || !fullCommitSHA.MatchString(revision) {
		return fmt.Errorf("external action must use owner/repository@ followed by a full 40-character commit SHA")
	}
	segments := strings.Split(action, "/")
	if len(segments) < 2 {
		return fmt.Errorf("external action must use owner/repository@ followed by a full 40-character commit SHA")
	}
	for _, segment := range segments {
		if !actionSegment.MatchString(segment) || segment == "." || segment == ".." {
			return fmt.Errorf("external action path contains an invalid owner, repository or subpath segment")
		}
	}
	return nil
}

func nodeError(node *yaml.Node, format string, arguments ...any) error {
	message := fmt.Sprintf(format, arguments...)
	if node.Line > 0 {
		return fmt.Errorf("line %d, column %d: %s", node.Line, node.Column, message)
	}
	return errors.New(message)
}
