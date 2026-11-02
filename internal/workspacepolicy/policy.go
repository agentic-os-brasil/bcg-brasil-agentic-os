// Package workspacepolicy owns deterministic workspace and cross-case policy.
// These checks admit paths at hook time; they are not filesystem race confinement.
package workspacepolicy

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ResolveProspective resolves all existing ancestors, including symlink aliases
// for paths whose final file does not exist. Broken aliases deny.
func ResolveProspective(p string) (string, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	var suffix []string
	for {
		_, err = os.Lstat(p)
		if err == nil {
			r, e := filepath.EvalSymlinks(p)
			if e != nil {
				return "", e
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				r = filepath.Join(r, suffix[i])
			}
			return filepath.Clean(r), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", err
		}
		suffix = append(suffix, filepath.Base(p))
		p = parent
	}
}

func CasePathAllowed(root, p string) bool {
	if strings.TrimSpace(p) == "" || strings.ContainsAny(p, "\x00\r\n") {
		return false
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, filepath.FromSlash(p))
	}
	target, err := ResolveProspective(p)
	if err != nil {
		return false
	}
	physicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	canonicalAccounts := filepath.Join(physicalRoot, "brain/accounts")
	accounts, err := ResolveProspective(filepath.Join(root, "brain/accounts"))
	if err != nil {
		return false
	}
	if !strings.EqualFold(accounts, canonicalAccounts) {
		return false
	}
	// A lexical path under accounts may not escape through an alias. External
	// aliases into accounts still undergo physical case classification below.
	lexical, e := filepath.Abs(p)
	if e != nil {
		return false
	}
	lexicalRoot, e := filepath.Abs(root)
	if e != nil {
		return false
	}
	inside := func(base, path string) bool {
		b := strings.ToLower(filepath.Clean(base))
		q := strings.ToLower(filepath.Clean(path))
		return q == b || strings.HasPrefix(q, b+string(filepath.Separator))
	}
	if inside(filepath.Join(lexicalRoot, "brain/accounts"), lexical) && !inside(accounts, target) {
		return false
	}
	rel, err := filepath.Rel(accounts, target)
	if err != nil {
		return false
	}
	parts := strings.Split(strings.ToLower(filepath.ToSlash(rel)), "/")
	if len(parts) < 3 || parts[0] == ".." || parts[1] != "cases" {
		return true
	}
	targetID := parts[0] + "/" + parts[2]
	active, err := readMarker(filepath.Join(accounts, ".active"))
	if err != nil || active == "" {
		return false
	}
	if targetID == active {
		return true
	}
	pending, err := readMarker(filepath.Join(accounts, ".pending"))
	return err == nil && pending == targetID
}

func readMarker(p string) (string, error) {
	physical, err := filepath.EvalSymlinks(p)
	if err != nil || !strings.EqualFold(physical, filepath.Clean(p)) {
		return "", errors.New("aliased marker")
	}
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 1025))
	if err != nil || len(b) > 1024 {
		return "", errors.New("invalid marker")
	}
	s := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(string(b), "\uFEFF")))
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(s, " \t\r\n\\") || strings.HasPrefix(parts[0], ".") || strings.HasPrefix(parts[1], ".") {
		return "", errors.New("invalid marker")
	}
	return s, nil
}
