package workspacepolicy

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Tool checks filesystem-bearing fields. Grep.pattern is text, never a path.
// Ordinary commands still pass through the independent removal and host guards.
func Tool(workspace, cwd, tool string, raw json.RawMessage) error {
	root, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return errors.New("workspace_unavailable")
	}
	current, err := filepath.EvalSymlinks(cwd)
	if err != nil || !filepath.IsAbs(cwd) || !Within(root, current) {
		return errors.New("cwd_outside_workspace")
	}
	var input map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &input) != nil || input == nil {
		return errors.New("tool_input_invalid")
	}
	pathKeys := []string{}
	switch tool {
	case "Read", "Write", "Edit", "MultiEdit":
		pathKeys = []string{"file_path"}
	case "NotebookRead", "NotebookEdit":
		pathKeys = []string{"notebook_path"}
	case "Glob", "Grep":
		pathKeys = []string{"path"}
	case "apply_patch":
		var patch string
		for _, key := range []string{"command", "patch"} {
			if value, ok := input[key]; ok {
				if json.Unmarshal(value, &patch) != nil {
					return errors.New("patch_invalid")
				}
				if patch != "" {
					break
				}
			}
		}
		if !strings.HasPrefix(strings.TrimSpace(patch), "*** Begin Patch") || !strings.HasSuffix(strings.TrimSpace(patch), "*** End Patch") {
			return errors.New("patch_invalid")
		}
		count := 0
		for _, line := range strings.Split(patch, "\n") {
			for _, prefix := range []string{"*** Add File: ", "*** Update File: ", "*** Delete File: ", "*** Move to: "} {
				if strings.HasPrefix(line, prefix) {
					count++
					if err := checkPath(root, current, strings.TrimPrefix(line, prefix)); err != nil {
						return err
					}
				}
			}
		}
		if count == 0 {
			return errors.New("patch_invalid")
		}
	}
	for _, key := range []string{"file_path", "path", "notebook_path"} {
		if value, ok := input[key]; ok {
			var path string
			if json.Unmarshal(value, &path) != nil || path == "" {
				return errors.New("path_invalid")
			}
			if err := checkPath(root, current, path); err != nil {
				return err
			}
		}
	}
	base := current
	for _, key := range pathKeys {
		value, ok := input[key]
		var path string
		if ok && json.Unmarshal(value, &path) != nil {
			return errors.New("path_invalid")
		}
		if path == "" {
			if tool == "Glob" || tool == "Grep" {
				continue
			}
			return errors.New("path_required")
		}
		if err := checkPath(root, current, path); err != nil {
			return err
		}
		if tool == "Glob" || tool == "Grep" {
			base = path
			if !filepath.IsAbs(base) {
				base = filepath.Join(current, base)
			}
		}
	}
	key := ""
	if tool == "Glob" {
		key = "pattern"
	} else if tool == "Grep" {
		key = "glob"
	}
	if key != "" {
		value, ok := input[key]
		var pattern string
		if !ok && tool == "Glob" {
			return errors.New("glob_required")
		}
		if ok {
			if json.Unmarshal(value, &pattern) != nil || pattern == "" {
				return errors.New("glob_invalid")
			}
			if err := checkGlob(root, base, pattern); err != nil {
				return err
			}
		}
	}
	if tool == "Grep" {
		return searchTree(root, base)
	}
	return nil
}
func Within(root, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
func checkPath(root, cwd, path string) error {
	if path == "" || strings.ContainsAny(path, "\x00\r\n\\") {
		return errors.New("path_invalid")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	target, err := ResolveProspective(path)
	if err != nil || !Within(root, target) || !CasePathAllowed(root, path) {
		return errors.New("path_outside_scope")
	}
	return nil
}
func checkGlob(root, base, pattern string) error {
	// Unsupported expansion, escapes and parent segments fail policy admission.
	if strings.ContainsAny(pattern, "{}[]\\\x00\r\n") {
		return errors.New("glob_unsupported")
	}
	for _, part := range strings.Split(filepath.ToSlash(pattern), "/") {
		if part == ".." {
			return errors.New("glob_outside_scope")
		}
	}
	joined := pattern
	if !filepath.IsAbs(joined) {
		joined = filepath.Join(base, joined)
	}
	prefix := joined
	if i := strings.IndexAny(prefix, "*?"); i >= 0 {
		prefix = filepath.Dir(prefix[:i])
	}
	if err := checkPath(root, base, filepath.ToSlash(prefix)); err != nil {
		return err
	}
	if !strings.ContainsAny(pattern, "*?") {
		return checkPath(root, base, filepath.ToSlash(joined))
	}
	return searchTree(root, prefix)
}

// Search paths may hide aliases or cross-case descendants even without a glob.
// The bounded walk does not follow aliases and makes no post-check race claim.
func searchTree(root, prefix string) error {
	resolved, err := filepath.EvalSymlinks(prefix)
	if errors.Is(err, os.ErrNotExist) {
		return checkPath(root, root, filepath.ToSlash(prefix))
	}
	if err != nil || !Within(root, resolved) {
		return errors.New("search_scope_unavailable")
	}
	prefix = resolved
	// Bound the traversable search tree, including aliases hidden after **.
	count := 0
	err = filepath.WalkDir(prefix, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			info, err := os.Stat(path)
			if err != nil || info.IsDir() {
				return errors.New("search_directory_alias_unsupported")
			}
		}
		count++
		if count > 4096 {
			return errors.New("glob_scope_too_large")
		}
		return checkPath(root, prefix, filepath.ToSlash(path))
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("glob_scope_unavailable")
	}
	return nil
}
