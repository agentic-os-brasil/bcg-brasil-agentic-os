package packaging

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

// CodePilotBinding carries one explicit local operator review selection. It
// grants neither native qualification nor publisher or installation authority.
type CodePilotBinding struct {
	UserHome           string                     `json:"user_home"`
	Workspace          string                     `json:"workspace"`
	SelectionSHA256    string                     `json:"selection_sha256"`
	OwnerReviewChannel string                     `json:"owner_review_channel"`
	Runtime            *EngineeringRuntimeBinding `json:"runtime,omitempty"`
}

func (p CodePilotBinding) validate() error {
	if p.OwnerReviewChannel != "local-browser" || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(p.SelectionSHA256) {
		return errors.New("explicit Code pilot owner review unavailable")
	}
	for _, path := range []string{p.UserHome, p.Workspace} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n") || strings.Contains(path, "${") {
			return errors.New("exact Code pilot selection required")
		}
	}
	if p.Runtime != nil {
		return p.Runtime.validate()
	}
	return nil
}
func prepareCodePilot(o *Options) error {
	if !o.CodePilot {
		return nil
	}
	if o.DesktopPilot || o.EngineeringRuntimePilot || o.Carrier != nil || o.Binding.InstalledManifestSHA256 != "" || o.Binding.InstalledClientInputsSHA256 != "" || o.Binding.OwnerReviewChannel != "local-browser" {
		return errors.New("Code pilot requires explicit local-browser and unenrolled experimental inputs")
	}
	if _, err := os.Lstat(filepath.Join(o.Binding.UserHome, ".maestro-install")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("Code pilot cannot use an enrolled installation")
	}
	selection, err := nativeSelectionFingerprint(o.Binding)
	if err != nil {
		return err
	}
	p := &CodePilotBinding{UserHome: o.Binding.UserHome, Workspace: o.Binding.WorkspacePath, SelectionSHA256: selection, OwnerReviewChannel: o.Binding.OwnerReviewChannel}
	if o.Binding.CLIVersion != "" || o.Binding.AuthorityRegistryPath != "" || o.Binding.AuthorityRegistrySHA256 != "" {
		if o.Version != o.Binding.CLIVersion {
			return errors.New("engineering Code version differs from constructor")
		}
		binding, err := preparedEngineeringRuntime(o.Binding, o.Server)
		if err != nil {
			return err
		}
		p.Runtime = &binding
	}
	if err = p.validate(); err != nil {
		return err
	}
	o.codePilot = p
	return nil
}

// ValidateSelection preserves existing hook fingerprints while checking the
// channel only for the foreground MCP server; hooks acquire no review channel.
func (p CodePilotBinding) ValidateSelection(o product.Options, serving bool) error {
	if err := p.validate(); err != nil {
		return err
	}
	expected := ""
	if serving {
		expected = p.OwnerReviewChannel
	}
	if o.UserHome != p.UserHome || o.WorkspacePath != p.Workspace || o.OwnerReviewChannel != expected {
		return errors.New("Code pilot owner review selection mismatch")
	}
	if _, err := os.Lstat(filepath.Join(p.UserHome, ".maestro-install")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("Code pilot cannot use an enrolled installation")
	}
	actual, err := nativeSelectionFingerprint(o)
	if err != nil || actual != p.SelectionSHA256 {
		return errors.New("Code pilot owner/workspace binding changed")
	}
	if p.Runtime != nil {
		return p.Runtime.ValidateSelection(o)
	}
	if o.CLIVersion != "" || o.AuthorityRegistryPath != "" || o.AuthorityRegistrySHA256 != "" {
		return errors.New("unseeded Code pilot cannot acquire a versioned runtime")
	}
	return nil
}
func codePilotMetadata(files map[string][]byte, platform string, p *CodePilotBinding) error {
	if p == nil {
		return nil
	}
	if err := p.validate(); err != nil {
		return err
	}
	var manifest map[string]any
	if err := json.Unmarshal(files[".claude-plugin/plugin.json"], &manifest); err != nil {
		return err
	}
	manifest["description"] = "Explicit local experimental Maestro Code pilot; protected changes use complete owner review in this computer's browser."
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	files[".claude-plugin/plugin.json"] = append(body, '\n')
	args := []string{"serve", "--packet", "--product-root", "${CLAUDE_PLUGIN_ROOT}", "--user-home", p.UserHome, "--workspace", p.Workspace, "--owner-review-channel", p.OwnerReviewChannel}
	body, err = json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"maestro": map[string]any{"type": "stdio", "command": "${CLAUDE_PLUGIN_ROOT}/scripts/" + binaryName(platform), "args": args}}}, "", "  ")
	if err != nil {
		return err
	}
	files[".mcp.json"] = append(body, '\n')
	files["README-PACKET.md"] = append(files["README-PACKET.md"], []byte("\nExplicit experimental Code pilot: owner review channel local-browser. The exact Owner directory and Workspace are fixed in this participant-local packet; do not share it. Protected foreground requests open the complete current/proposed record in the browser even when Code advertises native forms. Read the complete record before selecting the initially unchecked approval checkbox. Decline, closing, cancellation, expiry or browser failure gives no approval. A previous native form response cannot approve this channel. Native qualification fingerprints remain unchanged; browser review does not qualify native form readability, hooks, Desktop UI or production distribution.\n")...)
	return nil
}

// ValidateEngineeringCodePacketSelection reuses the existing prepared EPRT
// identity; scripts/ is the fixed Code image role, not a selected source path.
func ValidateEngineeringCodePacketSelection(root string, status PacketStatus, executingPath, executingSHA256, version, registrySHA256 string) error {
	if status.CodePilot == nil || status.CodePilot.Runtime == nil || status.Stable != nil {
		return errors.New("explicit engineering Code carrier required")
	}
	b := status.CodePilot.Runtime
	if err := b.validate(); err != nil {
		return err
	}
	expected := filepath.Join(root, "scripts", binaryName(status.Target.Platform()))
	pathMatches := expected == executingPath
	if status.Target.OS == "windows" {
		pathMatches = strings.EqualFold(expected, executingPath)
	}
	if !pathMatches || executingSHA256 != status.ServerSHA256 || b.ServerSHA256 != executingSHA256 || status.Version != version || b.ClientVersion != version || b.RegistrySHA256 != registrySHA256 || status.RegistrySHA256 != registrySHA256 || b.Target != status.Target {
		return errors.New("engineering Code executing image or authority differs")
	}
	if _, err := os.Lstat(filepath.Join(status.CodePilot.UserHome, ".maestro-install")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("engineering Code cannot use an enrolled installation")
	}
	return nil
}
func validateCodePilotInventory(p *CodePilotBinding, status PacketStatus, lifecycle *lifecyclePacketBinding, read func(string, int64) ([]byte, error)) error {
	if p == nil {
		return nil
	}
	if err := p.validate(); err != nil {
		return err
	}
	if status.Format != "plugin" || status.Stable != nil {
		return errors.New("Code pilot requires the experimental Code carrier")
	}
	if lifecycle != nil && (lifecycle.UserHome != p.UserHome || lifecycle.Workspace != p.Workspace) {
		return errors.New("Code pilot differs from lifecycle selection")
	}
	if status.NativeBindingSHA256 != "" && status.NativeBindingSHA256 != p.SelectionSHA256 {
		return errors.New("Code pilot differs from native selection")
	}
	if p.Runtime == nil {
		if status.RegistrySHA256 != "" {
			return errors.New("unseeded Code pilot cannot carry release authority")
		}
		return nil
	}
	b := p.Runtime
	if b.ClientVersion != status.Version || b.ServerSHA256 != status.ServerSHA256 || b.RegistrySHA256 != status.RegistrySHA256 || b.Target != status.Target {
		return errors.New("engineering Code inventory identities differ")
	}
	body, err := read("managed/trust/release-authority-registry.json", 1<<20)
	if err != nil {
		return err
	}
	expires, err := engineeringRuntimeAuthority(body, b.RegistrySHA256, b.Issuer, b.KeyID, time.Now())
	if err != nil || expires.UTC().Format(time.RFC3339) != b.ExpiresAt {
		return errors.New("engineering Code authority expired or changed; request a newly reviewed kit")
	}
	return nil
}
