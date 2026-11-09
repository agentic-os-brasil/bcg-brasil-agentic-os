// Package claudeagents projects Maestro's managed specialist contracts into
// Claude Code's project-native subagent directory. It never projects Maestro:
// SessionStart owns the user-facing hub identity.
package claudeagents

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspacepolicy"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
)

const managedMarker = "<!-- BCGOS:MANAGED-CLAUDE-AGENT -->\n"

type Status struct {
	State  string   `json:"state"`
	Path   string   `json:"path"`
	Agents []string `json:"agents"`
}

type definition struct {
	ID, Description, Tools, PermissionMode string
}

var managed = []definition{
	{ID: "case-agent", Description: "Executes one Maestro-authorized case packet inside its exact workspace scope; never delegates.", Tools: "Read, Write, Edit, Glob, Grep", PermissionMode: "default"},
	{ID: "client-account-agent", Description: "Client account strategic framing and stakeholder validation. Invoke only when Maestro selects the account route.", Tools: "[]", PermissionMode: "plan"},
	{ID: "darwin", Description: "Maestro system-health and bounded-housekeeping specialist; never handles client work.", Tools: "[]", PermissionMode: "plan"},
	{ID: "gamma-guardian", Description: "Tool-free analysis of one Maestro-supplied bounded quality packet; inspection and longitudinal custody are unavailable.", Tools: "[]", PermissionMode: "plan"},
	{ID: "pa-expert", Description: "Read-only functional or industrial PA Expert consulted only through Maestro.", Tools: "[]", PermissionMode: "plan"},
	{ID: "yoda", Description: "Calm owner-self proxy and senior refiner for high-leverage outputs selected by Maestro.", Tools: "[]", PermissionMode: "plan"},
}

const PluginNamespace = "maestro-product"

// Projection is inert native content; generating it never installs or admits a host.
type Projection struct {
	ID   string
	Name string
	Path string
	Body []byte
}

// NativeAgentID maps only the admitted plugin namespace and known specialists.
func NativeAgentID(id, namespace string) (string, error) {
	if namespace != "" && namespace != PluginNamespace {
		return "", errors.New("unsupported native agent namespace")
	}
	if !managedAgentTypes[id] {
		return "", errors.New("unknown native specialist")
	}
	if namespace == "" {
		return id, nil
	}
	return namespace + ":" + id, nil
}

// CanonicalAgentID recognizes exact workspace or product specialist names.
// It does not normalize arbitrary prefixes, whitespace, aliases or unknown IDs.
func CanonicalAgentID(name string) (string, bool) {
	id := strings.TrimPrefix(name, PluginNamespace+":")
	if !managedAgentTypes[id] {
		return "", false
	}
	return id, true
}

// Project is the deterministic shared workspace/plugin projection. Maestro
// stays the main-session hub. Namespace changes invocations, never identity data.
func Project(namespace string) ([]Projection, error) {
	if namespace != "" && namespace != PluginNamespace {
		return nil, errors.New("unsupported native agent namespace")
	}
	result := make([]Projection, 0, len(managed))
	for _, item := range managed {
		name, err := NativeAgentID(item.ID, namespace)
		if err != nil {
			return nil, err
		}
		body, err := render(item)
		if err != nil {
			return nil, err
		}
		body, err = ProjectAgentInvocations(body, namespace)
		if err != nil {
			return nil, err
		}
		result = append(result, Projection{ID: item.ID, Name: name, Path: "agents/" + item.ID + ".md", Body: body})
	}
	return result, nil
}

var executableAgentReference = regexp.MustCompile("(Agent\\(\\s*subagent_type\\s*=\\s*[\"']|Agent\\s+tool,\\s*`subagent_type:\\s*[\"'])([^\"'\\r\\n]+)([\"'])")

// ProjectAgentInvocations rewrites explicit Agent calls and the bundled methods'
// "Agent tool, `subagent_type: ...`" instruction. Canonical JSON, paths and
// skill references remain byte-identical.
func ProjectAgentInvocations(body []byte, namespace string) ([]byte, error) {
	if namespace != "" && namespace != PluginNamespace {
		return nil, errors.New("unsupported native agent namespace")
	}
	var projectionErr error
	projected := executableAgentReference.ReplaceAllFunc(body, func(call []byte) []byte {
		parts := executableAgentReference.FindSubmatch(call)
		id, ok := CanonicalAgentID(string(parts[2]))
		if !ok {
			if strings.HasPrefix(string(parts[2]), PluginNamespace+":") {
				projectionErr = errors.New("unknown product native specialist")
			}
			return call
		}
		name, err := NativeAgentID(id, namespace)
		if err != nil {
			projectionErr = err
			return call
		}
		return []byte(string(parts[1]) + name + string(parts[3]))
	})
	return projected, projectionErr
}

func Install(workspace string) (Status, error) {
	projections, err := Project("")
	if err != nil {
		return Status{}, err
	}
	root, rootPath, err := openAgentRoot(workspace, true)
	if err != nil {
		return Status{}, err
	}
	defer root.Close()
	if err := validateManaged(root, "replace"); err != nil {
		return Status{}, err
	}
	status := Status{State: "installed", Path: rootPath}
	for _, item := range projections {
		body := item.Body
		name := item.ID + ".md"
		current, readErr := readRegular(root, name)
		if readErr == nil && bytes.Equal(current, body) {
			status.Agents = append(status.Agents, item.ID)
			continue
		} else if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return Status{}, readErr
		}
		if err := writeAtomic(root, name, body); err != nil {
			return Status{}, err
		}
		status.Agents = append(status.Agents, item.ID)
	}
	return status, nil
}

func ValidateInstall(workspace string) error {
	root, _, err := openAgentRoot(workspace, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	return validateManaged(root, "replace")
}

func Inspect(workspace string) (Status, error) {
	projections, err := Project("")
	if err != nil {
		return Status{}, err
	}
	root, rootPath, err := openAgentRoot(workspace, false)
	status := Status{State: "installed", Path: rootPath}
	if errors.Is(err, os.ErrNotExist) {
		status.State = "absent"
		return status, nil
	}
	if err != nil {
		return Status{}, err
	}
	defer root.Close()
	if err := validateManaged(root, "inspect"); err != nil {
		return Status{}, err
	}
	for _, item := range projections {
		expected := item.Body
		body, err := readRegular(root, item.ID+".md")
		if errors.Is(err, os.ErrNotExist) {
			status.State = "absent"
			return status, nil
		}
		if err != nil {
			return Status{}, err
		}
		if !bytes.Equal(body, expected) {
			return Status{}, fmt.Errorf("Claude agent %s does not match the managed contract", item.ID)
		}
		status.Agents = append(status.Agents, item.ID)
	}
	return status, nil
}

func Uninstall(workspace string) (Status, error) {
	root, rootPath, err := openAgentRoot(workspace, false)
	status := Status{State: "absent", Path: rootPath}
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return Status{}, err
	}
	defer root.Close()
	if err := validateManaged(root, "remove"); err != nil {
		return Status{}, err
	}
	for _, item := range managed {
		name := item.ID + ".md"
		body, err := readRegular(root, name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return Status{}, err
		}
		if !owned(body, item) {
			return Status{}, fmt.Errorf("Claude agent path %s is user-owned; refusing to remove it", filepath.Join(rootPath, name))
		}
		if err := root.Remove(name); err != nil {
			return Status{}, err
		}
	}
	return status, nil
}

func ValidateUninstall(workspace string) error {
	root, _, err := openAgentRoot(workspace, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	return validateManaged(root, "remove")
}

func render(item definition) ([]byte, error) {
	contract, err := baseagents.Definition(item.ID)
	if err != nil {
		return nil, err
	}
	header := nativeHeader(item) + managedMarker + "\n"
	profile := "## Active native projection\n\nThis projection uses the bounded sequential maestro_planner profile: Maestro is the main-session hub, one specialist at a time, no child delegation. native_advisory is unavailable. The canonical contract below describes additional capabilities and historical adapters; their mention grants no authority in this profile. Use only the tools explicitly listed in frontmatter and the bounded packet supplied by Maestro. Native stop events do not verify a delivered artifact, review verdict or completed execution contract.\n\n"
	if item.ID == "gamma-guardian" {
		profile += "Gamma is tool-free bounded packet analysis only. Workspace/source-head inspection grants and longitudinal custody are unavailable (Spec043 remains partially unimplemented). Return UNAVAILABLE/BLOCKED for evidence requiring inspection; never invent source verification, signed DoneContract satisfaction or durable custody.\n\n"
		template, err := baseagents.Template("quality_guardian")
		if err != nil {
			return nil, err
		}
		profile += string(template) + "\n\n"
	}
	return append([]byte(header+profile), contract...), nil
}

func nativeHeader(item definition) string {
	return "---\nname: " + item.ID + "\ndescription: " + item.Description + "\ntools: " + item.Tools + "\npermissionMode: " + item.PermissionMode + "\n---\n\n"
}

func owned(body []byte, item definition) bool {
	current, err := render(item)
	if err != nil {
		return false
	}
	if bytes.Equal(body, current) {
		return true
	}
	// Only the five original specialists had the marker-before-frontmatter layout.
	if item.ID == "gamma-guardian" {
		return false
	}
	contract, err := baseagents.Definition(item.ID)
	if err != nil {
		return false
	}
	legacy := append([]byte(managedMarker+nativeHeader(item)), contract...)
	return bytes.Equal(body, legacy)
}

func writeAtomic(root *os.Root, name string, body []byte) error {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	temporaryName := fmt.Sprintf(".bcgos-agent-%x.tmp", random)
	temporary, err := root.OpenFile(temporaryName, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return root.Rename(temporaryName, name)
}

func openAgentRoot(workspace string, create bool) (*os.Root, string, error) {
	workspaceReal, err := filepath.EvalSymlinks(filepath.Clean(workspace))
	if err != nil {
		return nil, filepath.Join(workspace, ".claude", "agents"), err
	}
	workspaceReal, err = filepath.Abs(workspaceReal)
	if err != nil {
		return nil, "", err
	}
	workspaceRoot, err := os.OpenRoot(workspaceReal)
	if err != nil {
		return nil, "", err
	}
	defer workspaceRoot.Close()
	if err := ensurePlainDirectory(workspaceRoot, ".claude", create); err != nil {
		return nil, filepath.Join(workspaceReal, ".claude", "agents"), err
	}
	claudeRoot, err := workspaceRoot.OpenRoot(".claude")
	if err != nil {
		return nil, filepath.Join(workspaceReal, ".claude", "agents"), err
	}
	defer claudeRoot.Close()
	if err := ensurePlainDirectory(claudeRoot, "agents", create); err != nil {
		return nil, filepath.Join(workspaceReal, ".claude", "agents"), err
	}
	agentRoot, err := claudeRoot.OpenRoot("agents")
	return agentRoot, filepath.Join(workspaceReal, ".claude", "agents"), err
}

func ensurePlainDirectory(root *os.Root, name string, create bool) error {
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) && create {
		if err := root.Mkdir(name, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err = root.Lstat(name)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("Claude agent directory %s must be a real directory; refusing to follow it", name)
	}
	return nil
}

func readRegular(root *os.Root, name string) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("Claude agent path %s must be a regular file; refusing to follow it", name)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("Claude agent path %s changed during verification", name)
	}
	return io.ReadAll(file)
}

func validateManaged(root *os.Root, action string) error {
	for _, item := range managed {
		name := item.ID + ".md"
		body, err := readRegular(root, name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !owned(body, item) {
			return fmt.Errorf("Claude agent path %s is user-owned; refusing to %s it", name, action)
		}
	}
	return nil
}

var managedAgentTypes = map[string]bool{
	"case-agent": true, "client-account-agent": true, "yoda": true,
	"darwin": true, "pa-expert": true, "gamma-guardian": true,
}

func Managed(agentType string) bool { _, ok := CanonicalAgentID(agentType); return ok }

// GuardTool enforces the beta native-agent boundary independently of prompt
// instructions. No-tool specialists can never call a tool. Case Agent receives
// only local file tools and every explicit path is resolved inside the exact
// installed workspace, including existing symlink targets.
func GuardTool(agentType, toolName string, raw json.RawMessage, cwd, workspace string) (string, bool) {
	canonical, ok := CanonicalAgentID(agentType)
	if !ok {
		return "", false
	}
	agentType = canonical
	if agentType != "case-agent" {
		return "this Maestro specialist is tool-free; return only the bounded typed result to Maestro", true
	}
	allowed := map[string]bool{"Read": true, "Write": true, "Edit": true, "Glob": true, "Grep": true}
	if !allowed[toolName] {
		return "Case Agent may use only workspace-local file tools and cannot execute shell commands or delegate", true
	}
	if err := workspacepolicy.Tool(workspace, cwd, toolName, raw); err != nil {
		return "Case Agent path or pattern is outside the admitted workspace policy", true
	}
	return "", true
}
