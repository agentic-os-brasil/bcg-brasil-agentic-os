package atlasops

import (
	"errors"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type PageReference struct {
	Page      string `json:"page"`
	Reference string `json:"reference"`
	Target    string `json:"target"`
	Revision  string `json:"revision"`
}

var authoredReference = regexp.MustCompile(`\]\(([^)\r\n]*)\)`)
var opaqueAuthoredReference = regexp.MustCompile(`(?im)^ {0,3}\[[^\r\n]*\]:|<a\s[^>]*href\s*=|\[\[|<bcgos://atlas/`)

// Only the currently admitted authored corpus participates in this review.
func (engine *Engine) affectedReferences(names []string, selected string, recovery *PageProjection) ([]PageReference, error) {
	known := map[string]bool{}
	for _, name := range names {
		known[name] = true
	}
	refs := []PageReference{}
	physicalNames := map[string]string{}
	pages, bytes := 0, 0
	err := filepath.WalkDir(engine.root, func(full string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if full == engine.root {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(engine.root, full)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			parts := strings.Split(relative, "/")
			if len(parts) == 3 && parts[0] == "accounts" && parts[2] == "cases" {
				return nil
			}
			if engine.workspacePolicy.AdmitPage != nil && engine.workspacePolicy.AdmitPage(relative+"/index.md") != nil {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(relative, ".md") {
			return nil
		}
		if engine.workspacePolicy.AdmitPage != nil && engine.workspacePolicy.AdmitPage(relative) != nil {
			return nil
		}
		key := canonicalPageName(relative)
		if previous := physicalNames[key]; previous != "" && previous != relative {
			same, err := sameAtlasFile(engine.pagePath(previous), full)
			if err != nil {
				return err
			}
			if !same {
				return errors.New("admitted page names have an ambiguous case collision")
			}
		}
		physicalNames[key] = relative
		exact, folded := false, false
		for _, name := range names {
			exact = exact || name == relative
			folded = folded || canonicalPageName(name) == canonicalPageName(relative)
		}
		if folded && !exact {
			same, err := sameAtlasFile(engine.pagePath(selected), full)
			if err != nil {
				return err
			}
			if !same {
				return errors.New("admitted page names have an ambiguous case collision")
			}
		}
		var body string
		if recovery != nil && known[relative] {
			body, relative = recovery.Content, selected
		} else {
			body, err = engine.read(relative)
			if errors.Is(err, ErrPageRetired) || errors.Is(err, ErrLifecycleChanged) {
				return nil
			}
			if err != nil {
				return err
			}
		}
		pages++
		bytes += len(body)
		if pages > 1024 || bytes > 16<<20 {
			return errors.New("admitted reference corpus exceeds review bound")
		}
		if opaqueAuthoredReference.MatchString(body) {
			return errors.New("admitted reference syntax is outside the complete canonical review profile")
		}
		matches := authoredReference.FindAllStringSubmatch(body, -1)
		if strings.Count(body, "](") != len(matches) {
			return errors.New("admitted reference syntax cannot be completely reviewed")
		}
		for _, match := range matches {
			u, err := url.Parse(strings.TrimSpace(match[1]))
			if err != nil {
				return errors.New("admitted reference is opaque")
			}
			var target string
			if u.IsAbs() || u.Host != "" {
				prefix := "/workspace/" + engine.workspace.WorkspaceID + "/"
				if u.Scheme != "bcgos" || u.Host != "atlas" || !strings.HasPrefix(u.Path, prefix) {
					continue
				}
				target = strings.TrimPrefix(u.Path, prefix)
			} else if u.Path == "" {
				target = relative
			} else {
				target = path.Clean(path.Join(path.Dir(relative), u.Path))
			}
			if strings.ContainsAny(match[1], "(\t ") {
				return errors.New("admitted reference target is outside the canonical review profile")
			}
			affected := known[target]
			for name := range known {
				affected = affected || canonicalPageName(name) == canonicalPageName(target)
			}
			if affected || relative == selected {
				refs = append(refs, PageReference{Page: relative, Reference: match[1], Target: target, Revision: digest(body)})
				if len(refs) > 128 {
					return errors.New("affected references exceed complete review bound")
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, ref := range refs {
		if err := engine.admitWorkspacePage(ref.Page); err != nil {
			return nil, err
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Page == refs[j].Page {
			return refs[i].Reference < refs[j].Reference
		}
		return refs[i].Page < refs[j].Page
	})
	return refs, nil
}
