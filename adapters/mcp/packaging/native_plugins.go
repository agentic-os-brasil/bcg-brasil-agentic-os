package packaging

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

type nativeConfiguration struct {
	SHA256          string
	SessionSettings string
}

func managedNativePluginID(id string) bool {
	return regexp.MustCompile(`^maestro-product@[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`).MatchString(id)
}

// Registry data selects a source to inspect, never native execution authority.
// Read the same bytes the enclosing closure fingerprints; a raced replacement
// cannot supply parsed records from a different snapshot.
func (s *instructionSources) pluginRegistry(path string) ([]byte, error) {
	if err := s.file(path, false, 0); err != nil {
		return nil, err
	}
	if s.files[path] == "missing" {
		return nil, nil
	}
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return nil, sourceUnsupported("plugin_registry")
	}
	defer f.Close()
	if err := scheduler.ValidateSingleLink(f); err != nil {
		return nil, sourceUnsupported("plugin_registry")
	}
	body, err := io.ReadAll(io.LimitReader(f, (256<<10)+1))
	if err != nil || len(body) > 256<<10 {
		return nil, sourceUnsupported("plugin_registry")
	}
	info, err := f.Stat()
	if err != nil || s.files[path] != fmt.Sprintf("%s:%o", digest(body), info.Mode().Perm()) || packetJSON(body) != nil {
		return nil, sourceUnsupported("plugin_registry_changed")
	}
	return body, nil
}

func managedSourceIdentity(root string) (string, error) {
	f, err := scheduler.OpenDirectory(root)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return nativeSourceDirectoryIdentity(f)
}

func (s *instructionSources) managedPluginConfiguration(directory string, o Options) (string, error) {
	resolved := s.files["root:"+directory]
	if resolved == "" || resolved == "missing" {
		return "", sourceUnsupported("configuration_location")
	}
	// Plugins discovered through skills/synced roots have no supported installed
	// selection in this finite profile. Bind directory membership and refuse any
	// executable plugin manifest rather than silently dropping another origin.
	// Skills-directory plugins load from the actual user configuration root
	// and primary cwd, not every ancestor whose CLAUDE.md may be imported.
	for _, root := range []string{resolved, filepath.Join(o.Binding.WorkspacePath, ".claude")} {
		if err := s.refuseDiscoveredPlugins(filepath.Join(root, "skills"), true); err != nil {
			return "", err
		}
	}
	if err := s.refuseDiscoveredPlugins(filepath.Join(resolved, "plugins", "synced"), false); err != nil {
		return "", err
	}
	registry := filepath.Join(resolved, "plugins", "installed_plugins.json")
	body, err := s.pluginRegistry(registry)
	if err != nil {
		return "", err
	}
	if body == nil {
		if len(s.plugins) != 0 {
			return "", sourceUnsupported("plugin_registry")
		}
		return "", nil
	}
	var record struct {
		Version int `json:"version"`
		Plugins map[string][]struct {
			Scope       string `json:"scope"`
			InstallPath string `json:"installPath"`
			Version     string `json:"version"`
			ProjectPath string `json:"projectPath"`
		} `json:"plugins"`
	}
	if json.Unmarshal(body, &record) != nil || record.Version != 2 || len(record.Plugins) > 1 || len(s.plugins) > 1 {
		return "", sourceUnsupported("plugin_registry")
	}
	if len(record.Plugins) != len(s.plugins) {
		return "", sourceUnsupported("plugins")
	}
	disabled := map[string]bool{}
	for id, installs := range record.Plugins {
		if !managedNativePluginID(id) || !s.plugins[id] || len(installs) != 1 {
			return "", sourceUnsupported("plugins")
		}
		installed := installs[0]
		// Multiple settings/install scopes need a separately supported precedence
		// contract. The first finite profile admits one exact user installation.
		if installed.Scope != "user" || installed.ProjectPath != "" || !filepath.IsAbs(installed.InstallPath) || filepath.Clean(installed.InstallPath) != installed.InstallPath {
			return "", sourceUnsupported("plugin_scope")
		}
		identity, err := managedSourceIdentity(installed.InstallPath)
		if err != nil {
			return "", sourceUnsupported("plugin_source")
		}
		status, err := ValidatePacket(installed.InstallPath)
		if err != nil || status.Format != "plugin" || status.ServerSHA256 != digest(o.serverBody) || status.Version != installed.Version || status.Target != o.target || status.Stable != nil || status.DesktopPilot != nil || (status.NativeMode != "mcp_only" && status.NativeMode != "mcp_lifecycle" && status.NativeMode != "native") {
			return "", sourceUnsupported("plugin_inventory")
		}
		inventoryBytes, err := directoryPacketSource(installed.InstallPath).read(InventoryName, 1<<20)
		if err != nil || digest(inventoryBytes) != status.InventorySHA256 {
			return "", sourceUnsupported("plugin_inventory_changed")
		}
		var inventory packetInventory
		if json.Unmarshal(inventoryBytes, &inventory) != nil {
			return "", sourceUnsupported("plugin_inventory")
		}
		if inventory.Lifecycle != nil && (inventory.Lifecycle.UserHome != o.Binding.UserHome || inventory.Lifecycle.Workspace != o.Binding.WorkspacePath) {
			return "", sourceUnsupported("plugin_binding")
		}
		if after, err := managedSourceIdentity(installed.InstallPath); err != nil || after != identity {
			return "", sourceUnsupported("plugin_source_changed")
		}
		s.files["managed-plugin:"+id] = installed.InstallPath + ":" + identity + ":" + status.InventorySHA256 + ":" + status.ServerSHA256
		disabled[id] = false
	}
	if len(disabled) == 0 {
		return "", nil
	}
	override, err := json.Marshal(map[string]any{"enabledPlugins": disabled})
	if err != nil {
		return "", err
	}
	s.files["qualification:session-settings"] = digest(override)
	return string(override), nil
}

func (s *instructionSources) refuseDiscoveredPlugins(root string, skills bool) error {
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		s.files["plugin-discovery:"+root] = "missing"
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || scheduler.ValidateDirectory(root) != nil {
		return sourceUnsupported("plugin_discovery")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) > 256 {
		return sourceUnsupported("plugin_discovery")
	}
	members := []string{}
	for _, entry := range entries {
		members = append(members, entry.Name()+":"+entry.Type().String())
		if entry.Type()&os.ModeSymlink != 0 {
			return sourceUnsupported("plugin_discovery")
		}
		if !skills {
			return sourceUnsupported("synced_plugins")
		}
		if !entry.IsDir() {
			continue
		}
		manifest := filepath.Join(root, entry.Name(), ".claude-plugin", "plugin.json")
		if _, err := os.Lstat(manifest); !errors.Is(err, os.ErrNotExist) {
			return sourceUnsupported("discovered_plugins")
		}
	}
	sort.Strings(members)
	body, _ := json.Marshal(members)
	s.files["plugin-discovery:"+root] = digest(body)
	return nil
}
