// Package claudeagents projects Maestro's managed specialist contracts into
// Claude Code's project-native agent directory. Direct repository enrollment
// may additionally project a main-session frontend; ordinary Hub installation
// still projects specialists only.
package claudeagents

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
)

const managedMarker = "<!-- BCGOS:MANAGED-CLAUDE-AGENT -->\n"
const DirectHubID = "maestro-hub"

type Status struct {
	State  string   `json:"state"`
	Path   string   `json:"path"`
	Agents []string `json:"agents"`
}

type definition struct {
	ID, Description, Tools, PermissionMode string
	Skills                                 []string
	DirectHub                              bool
}

var directHub = definition{
	ID:          DirectHubID,
	Description: "Main-session frontend for the governed Maestro operating layer in an enrolled repository or worktree.",
	Skills:      []string{"maestro-operator"},
	DirectHub:   true,
}

var managed = []definition{
	{ID: "client-account-agent", Description: "Client account strategic framing and stakeholder validation. Invoke only when Maestro selects the account route.", Tools: "[]", PermissionMode: "plan"},
	{ID: "case-agent", Description: "Executes one Maestro-authorized case packet inside its exact workspace scope; never delegates.", Tools: "Read, Write, Edit, Glob, Grep", PermissionMode: "default"},
	{ID: "yoda", Description: "Calm owner-self proxy and senior refiner for high-leverage outputs selected by Maestro.", Tools: "[]", PermissionMode: "plan"},
	{ID: "darwin", Description: "Maestro system-health and bounded-housekeeping specialist; never handles client work.", Tools: "[]", PermissionMode: "plan"},
	{ID: "pa-expert", Description: "Read-only functional or industrial PA Expert consulted only through Maestro.", Tools: "[]", PermissionMode: "plan"},
}

func Install(workspace string) (Status, error) {
	root, rootPath, err := openAgentRoot(workspace, true)
	if err != nil {
		return Status{}, err
	}
	defer root.Close()
	if err := validateManaged(root, "replace"); err != nil {
		return Status{}, err
	}
	status := Status{State: "installed", Path: rootPath}
	for _, item := range managed {
		body, err := render(item)
		if err != nil {
			return Status{}, err
		}
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

// InstallDirectHub projects only the direct-entry main-session frontend.
func InstallDirectHub(workspace string) (Status, error) {
	root, rootPath, err := openAgentRoot(workspace, true)
	if err != nil {
		return Status{}, err
	}
	defer root.Close()
	if err := validateDefinitions(root, []definition{directHub}, "replace"); err != nil {
		return Status{}, err
	}
	body, err := render(directHub)
	if err != nil {
		return Status{}, err
	}
	name := DirectHubID + ".md"
	current, readErr := readRegular(root, name)
	if readErr == nil && bytes.Equal(current, body) {
		return Status{State: "installed", Path: rootPath, Agents: []string{DirectHubID}}, nil
	}
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return Status{}, readErr
	}
	if err := writeAtomic(root, name, body); err != nil {
		return Status{}, err
	}
	return Status{State: "installed", Path: rootPath, Agents: []string{DirectHubID}}, nil
}

func ValidateDirectHubInstall(workspace string) error {
	root, _, err := openAgentRoot(workspace, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	return validateDefinitions(root, []definition{directHub}, "replace")
}

// DirectHubManaged reports ownership without treating a user-owned file with
// the reserved filename as removable Maestro state.
func DirectHubManaged(workspace string) (bool, error) {
	root, _, err := openAgentRoot(workspace, false)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer root.Close()
	body, err := readRegular(root, DirectHubID+".md")
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return hasManagedMarker(body), nil
}

func InspectDirectHub(workspace string) (Status, error) {
	root, rootPath, err := openAgentRoot(workspace, false)
	status := Status{State: "absent", Path: rootPath}
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return Status{}, err
	}
	defer root.Close()
	expected, err := render(directHub)
	if err != nil {
		return Status{}, err
	}
	body, err := readRegular(root, DirectHubID+".md")
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return Status{}, err
	}
	if !bytes.Equal(body, expected) {
		if hasManagedMarker(body) {
			status.State = "outdated"
			return status, nil
		}
		return Status{}, fmt.Errorf("Claude agent %s does not match the managed contract", DirectHubID)
	}
	status.State = "installed"
	status.Agents = []string{DirectHubID}
	return status, nil
}

func UninstallDirectHub(workspace string) (Status, error) {
	root, rootPath, err := openAgentRoot(workspace, false)
	status := Status{State: "absent", Path: rootPath}
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return Status{}, err
	}
	defer root.Close()
	body, err := readRegular(root, DirectHubID+".md")
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return Status{}, err
	}
	if !hasManagedMarker(body) {
		return Status{}, fmt.Errorf("Claude agent path %s is user-owned; refusing to remove it", filepath.Join(rootPath, DirectHubID+".md"))
	}
	if err := root.Remove(DirectHubID + ".md"); err != nil {
		return Status{}, err
	}
	return status, nil
}

func Inspect(workspace string) (Status, error) {
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
	for _, item := range managed {
		expected, err := render(item)
		if err != nil {
			return Status{}, err
		}
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
		if !hasManagedMarker(body) {
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
	var contract []byte
	var err error
	if item.DirectHub {
		contract = baseruntime.DirectHubContract()
	} else {
		contract, err = baseagents.Definition(item.ID)
		if err != nil {
			return nil, err
		}
	}
	header := "---\nname: " + item.ID + "\ndescription: " + item.Description + "\n"
	if item.Tools != "" {
		header += "tools: " + item.Tools + "\n"
	}
	if item.PermissionMode != "" {
		header += "permissionMode: " + item.PermissionMode + "\n"
	}
	if len(item.Skills) > 0 {
		header += "skills:\n"
		for _, skill := range item.Skills {
			header += "  - " + skill + "\n"
		}
	}
	header += "---\n" + managedMarker + "\n"
	return append([]byte(header), contract...), nil
}

func hasManagedMarker(body []byte) bool {
	if bytes.HasPrefix(body, []byte(managedMarker)) {
		// Accept the unreleased legacy projection so explicit repair and removal
		// remain safe after moving the marker behind native YAML frontmatter.
		return true
	}
	if !bytes.HasPrefix(body, []byte("---\n")) {
		return false
	}
	frontmatterEnd := bytes.Index(body[len("---\n"):], []byte("\n---\n"))
	if frontmatterEnd < 0 {
		return false
	}
	afterFrontmatter := len("---\n") + frontmatterEnd + len("\n---\n")
	return bytes.HasPrefix(body[afterFrontmatter:], []byte(managedMarker))
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
	return validateDefinitions(root, managed, action)
}

func validateDefinitions(root *os.Root, definitions []definition, action string) error {
	for _, item := range definitions {
		name := item.ID + ".md"
		body, err := readRegular(root, name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !hasManagedMarker(body) {
			return fmt.Errorf("Claude agent path %s is user-owned; refusing to %s it", name, action)
		}
	}
	return nil
}

var managedAgentTypes = map[string]bool{
	"case-agent": true, "client-account-agent": true, "yoda": true,
	"darwin": true, "pa-expert": true,
}

func Managed(agentType string) bool { return managedAgentTypes[agentType] }

// GuardTool enforces the beta native-agent boundary independently of prompt
// instructions. No-tool specialists can never call a tool. Case Agent receives
// only local file tools and every explicit path is resolved inside the exact
// installed workspace, including existing symlink targets.
func GuardTool(agentType, toolName string, raw json.RawMessage, cwd, workspace string) (string, bool) {
	if !managedAgentTypes[agentType] {
		return "", false
	}
	if agentType != "case-agent" {
		return "this Maestro specialist is tool-free; return only the bounded typed result to Maestro", true
	}
	allowed := map[string]bool{"Read": true, "Write": true, "Edit": true, "Glob": true, "Grep": true}
	if !allowed[toolName] {
		return "Case Agent may use only workspace-local file tools and cannot execute shell commands or delegate", true
	}
	workspaceReal, err := filepath.EvalSymlinks(filepath.Clean(workspace))
	if err != nil {
		return "authorized Maestro workspace could not be resolved", true
	}
	workspaceReal, err = filepath.Abs(workspaceReal)
	if err != nil || strings.TrimSpace(cwd) == "" || !filepath.IsAbs(cwd) {
		return "Case Agent working directory is outside the authorized Maestro workspace", true
	}
	cwdReal, err := filepath.EvalSymlinks(filepath.Clean(cwd))
	if err != nil {
		return "Case Agent working directory could not be resolved inside the authorized Maestro workspace", true
	}
	cwdReal, err = filepath.Abs(cwdReal)
	if err != nil || !within(workspaceReal, cwdReal) {
		return "Case Agent working directory is outside the authorized Maestro workspace", true
	}
	var input map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &input) != nil {
		return "Case Agent tool input could not be verified against the workspace boundary", true
	}
	var pathValues []string
	for _, key := range []string{"file_path", "path", "notebook_path"} {
		if value, ok := input[key].(string); ok && strings.TrimSpace(value) != "" {
			pathValues = append(pathValues, value)
		}
	}
	if len(pathValues) == 0 {
		if toolName == "Glob" || toolName == "Grep" {
			return "", true
		}
		return "Case Agent file operation is missing an explicit workspace-local path", true
	}
	for _, pathValue := range pathValues {
		target := pathValue
		if !filepath.IsAbs(target) {
			target = filepath.Join(cwdReal, target)
		}
		target, err = resolveTarget(target)
		if err != nil || !within(workspaceReal, target) {
			return "Case Agent path crosses the authorized Maestro workspace boundary", true
		}
	}
	return "", true
}

func within(root, target string) bool {
	rootAbs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(filepath.Clean(target))
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(rootAbs, targetAbs)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func resolveTarget(target string) (string, error) {
	target, err := filepath.Abs(filepath.Clean(target))
	if err != nil {
		return "", err
	}
	candidate := target
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err == nil {
			for index := len(suffix) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, suffix[index])
			}
			return filepath.Abs(filepath.Clean(resolved))
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", err
		}
		suffix = append(suffix, filepath.Base(candidate))
		candidate = parent
	}
}
