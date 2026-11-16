package packaging

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

func TestDesktopPilotLocalOwnerReviewSelection(t *testing.T) {
	root := t.TempDir()
	pilot := &DesktopPilotBinding{WorkspaceID: strings.Repeat("a", 32), UserHome: filepath.Join(root, "owner"), Workspace: filepath.Join(root, "workspace"), OwnerReviewChannel: "local-browser"}
	if err := pilot.validate(); err != nil {
		t.Fatal(err)
	}
	files, _, err := portableMetadata("mcpb", "darwin", "0.0.0", nil, nil, pilot)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Description string `json:"description"`
		Server      struct {
			Config struct {
				Args []string `json:"args"`
			} `json:"mcp_config"`
		} `json:"server"`
		UserConfig map[string]any `json:"user_config"`
	}
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	args := manifest.Server.Config.Args
	if len(args) < 2 || args[len(args)-2] != "--owner-review-channel" || args[len(args)-1] != "local-browser" || len(manifest.UserConfig) != 0 {
		t.Fatalf("explicit startup choice missing: %+v", manifest)
	}
	if !strings.Contains(manifest.Description, "local browser") || !strings.Contains(string(files["README-PACKET.md"]), "Owner review channel: local-browser") {
		t.Fatal("owner channel not disclosed")
	}
	pilot.OwnerReviewChannel = "model-approved"
	if err := pilot.validate(); err == nil {
		t.Fatal("unknown approval channel admitted")
	}
	pilot.OwnerReviewChannel = "local-browser"
	if err := pilot.ValidateSelection(product.Options{UserHome: pilot.UserHome, WorkspacePath: pilot.Workspace, OwnerReviewChannel: "other"}); err == nil {
		t.Fatal("startup channel substitution admitted")
	}
	options := Options{Binding: product.Options{OwnerReviewChannel: "local-browser"}}
	if err := prepareDesktopPilot(&options, "mcpb"); err == nil {
		t.Fatal("generic packet acquired local operator opt-in")
	}
	generic, _, err := portableMetadata("mcpb", "darwin", "0.0.0", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(generic["manifest.json"]), "owner-review-channel") || strings.Contains(string(generic["README-PACKET.md"]), "Owner review channel:") {
		t.Fatal("generic packet silently enabled local browser")
	}
}
