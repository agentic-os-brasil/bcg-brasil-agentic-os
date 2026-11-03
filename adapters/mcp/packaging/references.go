package packaging

import (
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

type assetReference struct {
	Source           string `json:"source"`
	Target           string `json:"target"`
	State            string `json:"state"`
	ResolvedResource string `json:"resolved_resource,omitempty"`
}

var markdownLink = regexp.MustCompile(`\[[^\]\n]*\]\(([^)\s]+)(?:[ \t]+"[^"]*")?\)`)
var codeLiteral = regexp.MustCompile("`([^`\n]+)`")
var literalFile = regexp.MustCompile(`(?:^|[ \t])([A-Za-z0-9_./$-]+\.(?:md|json|yaml|yml|py|sh|ps1|exe|txt))\b`)

// Enumerate the reference forms used by immutable methods. A named absent
// helper is explicitly unavailable, never an executable hidden dependency.
// External links are source pointers and are not fetched by this builder.
func methodAssetReferences(methods product.MethodPackage, agents map[string][]byte) []assetReference {
	resources := map[string][]byte{}
	for _, m := range methods.Methods {
		for _, r := range m.Resources {
			resources[r.URI] = m.Files[r.Path]
		}
	}
	for name, body := range methods.Assets {
		uri := "maestro://methods/" + name
		if name == "methods-index.json" {
			uri = "maestro://methods/catalog.json"
		}
		resources[uri] = body
	}
	for name, body := range agents {
		resources["maestro://agents/"+name] = body
	}
	seen := map[string]bool{}
	var result []assetReference
	add := func(source, target string) {
		if target == "" || strings.HasPrefix(target, "#") {
			return
		}
		key := source + "\x00" + target
		if seen[key] {
			return
		}
		seen[key] = true
		ref := assetReference{Source: source, Target: target, State: "unavailable_in_packet"}
		parsed, err := url.Parse(target)
		if err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") {
			ref.State = "external_source_pointer"
		} else {
			base, _ := url.Parse(source)
			resolved := ""
			if err == nil && parsed.Scheme == "maestro" {
				resolved = target
			} else if err == nil && parsed.Scheme == "" && parsed.RawQuery == "" && !strings.HasPrefix(parsed.Path, "/") && !strings.ContainsAny(target, "%\\$") {
				resolved = "maestro://" + base.Host + path.Clean(path.Join(path.Dir(base.Path), parsed.Path))
			}
			if _, ok := resources[resolved]; ok {
				ref.State = "embedded_resource"
				ref.ResolvedResource = resolved
			}
		}
		result = append(result, ref)
	}
	for source, body := range resources {
		if !strings.HasSuffix(source, ".md") {
			continue
		}
		for _, match := range markdownLink.FindAllSubmatch(body, -1) {
			add(source, string(match[1]))
		}
		for _, literal := range codeLiteral.FindAllSubmatch(body, -1) {
			for _, file := range literalFile.FindAllSubmatch(literal[1], -1) {
				add(source, string(file[1]))
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Source != result[j].Source {
			return result[i].Source < result[j].Source
		}
		return result[i].Target < result[j].Target
	})
	return result
}
