// Package mermaiddoc validates the structural integrity of Mermaid blocks in
// repository Markdown. It deliberately performs an offline, dependency-free
// check; rendering remains the responsibility of the Markdown host.
package mermaiddoc

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var supportedDiagramTypes = map[string]bool{
	"architecture-beta":  true,
	"block-beta":         true,
	"classDiagram":       true,
	"erDiagram":          true,
	"flowchart":          true,
	"gantt":              true,
	"gitGraph":           true,
	"graph":              true,
	"journey":            true,
	"kanban":             true,
	"mindmap":            true,
	"packet-beta":        true,
	"pie":                true,
	"quadrantChart":      true,
	"requirementDiagram": true,
	"sankey-beta":        true,
	"sequenceDiagram":    true,
	"stateDiagram":       true,
	"stateDiagram-v2":    true,
	"timeline":           true,
	"xychart-beta":       true,
}

// Validate scans Markdown files beneath root and rejects malformed Mermaid
// fences before they reach a pull request.
func Validate(root string) error {
	var problems []error
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if shouldSkipDirectory(filepath.ToSlash(relative)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return nil
		}
		if err := validateFile(path); err != nil {
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				relative = path
			}
			problems = append(problems, fmt.Errorf("%s: %w", filepath.ToSlash(relative), err))
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("scan Markdown files: %w", err)
	}
	return errors.Join(problems...)
}

func shouldSkipDirectory(relative string) bool {
	return relative == ".git" ||
		relative == "node_modules" ||
		strings.HasPrefix(relative, "node_modules/") ||
		relative == ".claude/worktrees" ||
		strings.HasPrefix(relative, ".claude/worktrees/")
}

func validateFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open Markdown: %w", err)
	}
	defer file.Close()

	var problems []error
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	inMermaid := false
	openLine := 0
	diagramTypeFound := false
	for scanner.Scan() {
		lineNumber++
		trimmed := strings.TrimSpace(scanner.Text())
		if !inMermaid {
			if trimmed == "```mermaid" {
				inMermaid = true
				openLine = lineNumber
				diagramTypeFound = false
			}
			continue
		}

		if trimmed == "```" {
			if !diagramTypeFound {
				problems = append(problems, fmt.Errorf("line %d: Mermaid block must begin with a supported diagram type", openLine))
			}
			inMermaid = false
			continue
		}
		if diagramTypeFound || trimmed == "" || strings.HasPrefix(trimmed, "%%") {
			continue
		}
		diagramTypeFound = isSupportedDiagramHeader(trimmed)
		if !diagramTypeFound {
			problems = append(problems, fmt.Errorf("line %d: Mermaid block must begin with a supported diagram type", openLine))
			diagramTypeFound = true
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read Markdown: %w", err)
	}
	if inMermaid {
		problems = append(problems, fmt.Errorf("line %d: unclosed Mermaid block", openLine))
	}
	return errors.Join(problems...)
}

func isSupportedDiagramHeader(line string) bool {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false
	}
	return supportedDiagramTypes[fields[0]] || strings.HasPrefix(fields[0], "C4")
}
