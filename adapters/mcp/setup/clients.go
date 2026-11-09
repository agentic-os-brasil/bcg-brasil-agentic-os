package setup

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func stableCarrier(id string) packaging.StableCarrier {
	return packaging.StableCarrier{Profile: installer.StableClientProfile, Protocol: installer.StableClientProtocol, ID: id, Version: installer.StableCarrierVersion}
}

// ApplyClients always runs generators from the authenticated active server.
// The stable installer's older linked methods are never a packet source.
func ApplyClients(ctx context.Context, o installer.ClientOptions, digest string, confirmed bool) (installer.ClientRecord, error) {
	return installer.MaterializeClientsSeeded(ctx, o, digest, confirmed, CurrentClientMaterializer(o))
}

// CurrentClientMaterializer is trusted process composition at the installer
// seam. It returns verified files, never installation or native-host authority.
func CurrentClientMaterializer(o installer.ClientOptions) installer.ClientMaterializer {
	return func(ctx context.Context, s installtx.State, b installer.ClientBinding, old *installer.ClientRecord) (installer.ClientMaterialization, error) {
		serverName := "maestro-mcp"
		if s.TargetOS == "windows" {
			serverName += ".exe"
		}
		server := filepath.Join(o.ManagedRoot, "bin", serverName)
		identity, err := releasecontract.ReadServerIdentity(server)
		if err != nil {
			return installer.ClientMaterialization{}, err
		}
		if identity.Version != s.Server.Version || identity.EmbeddedInputsSHA256 != s.Server.EmbeddedInputsSHA256 {
			return installer.ClientMaterialization{}, errors.New("actual current packet generator identity changed")
		}
		expected := stableCarrier(o.Request.CarrierID)
		stage := o.Request.CodeRoot + ".candidate-" + o.Request.CarrierID + "-" + s.Binding.ManifestSHA256
		desktopStage := o.Request.DesktopPath + ".candidate-" + o.Request.CarrierID + "-" + s.Binding.ManifestSHA256 + ".mcpb"
		// A complete interrupted candidate is reusable only after exact inspection;
		// partial or foreign candidates remain preserved for operator diagnosis.
		if _, e := os.Lstat(stage); errors.Is(e, os.ErrNotExist) {
			if err = os.Mkdir(stage, 0700); err != nil {
				return installer.ClientMaterialization{}, err
			}
			if err = os.Mkdir(filepath.Join(stage, "plugins"), 0700); err != nil {
				return installer.ClientMaterialization{}, err
			}
			args := currentGeneratorArgs("plugin", filepath.Join(stage, "plugins", "maestro-product"), server, o, s)
			if _, err = clientCommand(ctx, server, args, b.Workspace); err != nil {
				return installer.ClientMaterialization{}, fmt.Errorf("actual current plugin generator: %w", err)
			}
			if err = writeMarketplaceAndLauncher(stage, o); err != nil {
				return installer.ClientMaterialization{}, err
			}
		} else if e != nil {
			return installer.ClientMaterialization{}, e
		}
		// The digest becomes admissible only after canonical inspection and an
		// unchanged observation. Never promote a fresh post-publication hash.
		expectedCode, err := installer.ClientTreeDigest(stage)
		if err != nil {
			return installer.ClientMaterialization{}, err
		}
		if err = inspectMarketplace(ctx, stage, o, identity, s.Server.SHA256); err != nil {
			return installer.ClientMaterialization{}, err
		}
		if err = requireInspectedClientTree(stage, expectedCode); err != nil {
			return installer.ClientMaterialization{}, err
		}
		if _, e := os.Lstat(desktopStage); errors.Is(e, os.ErrNotExist) {
			if _, err = clientCommand(ctx, server, currentGeneratorArgs("mcpb", desktopStage, server, o, s), b.Workspace); err != nil {
				return installer.ClientMaterialization{}, fmt.Errorf("actual current MCPB generator: %w", err)
			}
		} else if e != nil {
			return installer.ClientMaterialization{}, e
		}
		expectedDesktop, err := installer.ClientArchiveDigest(desktopStage)
		if err != nil {
			return installer.ClientMaterialization{}, err
		}
		desktopStatus, err := inspectCurrentPacket(ctx, desktopStage, "mcpb", o, identity, s.Server.SHA256)
		nativeContract := desktopStatus.NativeContractSHA256
		if err != nil {
			return installer.ClientMaterialization{}, err
		}
		if err = inspectDesktopCandidateBinding(desktopStage, o, s.TargetOS, nativeContract); err != nil {
			return installer.ClientMaterialization{}, err
		}
		if err = requireInspectedClientArchive(desktopStage, expectedDesktop); err != nil {
			return installer.ClientMaterialization{}, err
		}
		material := installer.ClientMaterialization{LifecycleContext: o.Request.LifecycleContext, Release: s.Release, ManifestSHA256: s.Binding.ManifestSHA256, ClientInputsSHA256: s.Binding.ClientInputsSHA256, ServerSHA256: s.Server.SHA256, EmbeddedInputsSHA256: s.Server.EmbeddedInputsSHA256, BundleSHA256: s.Binding.BundleSHA256, CodeRoot: o.Request.CodeRoot, DesktopPath: o.Request.DesktopPath, DesktopNativeContractSHA256: nativeContract, BootstrapServerSHA256: s.Server.SHA256, BootstrapEmbeddedInputsSHA256: s.Server.EmbeddedInputsSHA256}
		// First publication may encounter an already-swapped destination. Admit
		// its exact reviewed native contract before any Code or Desktop swap.
		if old == nil {
			if _, e := os.Lstat(o.Request.DesktopPath); e == nil {
				before, e := installer.ClientArchiveDigest(o.Request.DesktopPath)
				if e != nil {
					return material, e
				}
				if e = inspectCurrentDesktopContract(ctx, o.Request.DesktopPath, o, identity, s.Server.SHA256, nativeContract); e != nil {
					return material, e
				}
				if e = requireInspectedClientArchive(o.Request.DesktopPath, before); e != nil {
					return material, e
				}
				expectedDesktop = before
			} else if !errors.Is(e, os.ErrNotExist) {
				return material, e
			}
		}
		// A reviewed recovery may finish an authenticated current replacement
		// whose swap completed before its finite current record was published.
		adoptCurrent := false
		if old != nil && o.Request.Reconcile {
			sha, e := installer.ClientArchiveDigest(o.Request.DesktopPath)
			if e == nil && sha != old.Materialization.DesktopSHA256 {
				if e = inspectCurrentDesktopContract(ctx, o.Request.DesktopPath, o, identity, s.Server.SHA256, nativeContract); e != nil {
					return material, e
				}
				if e = requireInspectedClientArchive(o.Request.DesktopPath, sha); e != nil {
					return material, e
				}
				expectedDesktop = sha
				adoptCurrent = true
			}
		}
		if _, err = inspectDesktopBackup(o, old); err != nil {
			return material, err
		}
		reuse := old != nil && !o.Request.ManualDesktopUpdate && old.Materialization.DesktopNativeContractSHA256 == nativeContract && !adoptCurrent
		if reuse {
			m := old.Materialization
			if err = packaging.InspectReusedStableExtension(m.DesktopPath, expected, m.DesktopSHA256, m.BootstrapServerSHA256, m.BootstrapEmbeddedInputsSHA256, m.DesktopNativeContractSHA256); err != nil {
				return material, err
			}
			expectedDesktop = m.DesktopSHA256
			material.BootstrapServerSHA256 = m.BootstrapServerSHA256
			material.BootstrapEmbeddedInputsSHA256 = m.BootstrapEmbeddedInputsSHA256
		}
		// Preflight both visible slots before publishing either. There is no
		// admitted strong custody for replacing a shared client tree on this
		// profile; a changed existing slot needs an explicit manual client step.
		if err = preflightClientPublication(stage, desktopStage, o, reuse || adoptCurrent); err != nil {
			return material, err
		}
		if err = swapOwnedCode(ctx, stage, o, old); err != nil {
			return material, err
		}
		if !reuse && !adoptCurrent {
			if err = swapOwnedDesktop(ctx, desktopStage, o, old, identity, nativeContract); err != nil {
				return material, err
			}
		}
		// Preserve generated candidates, including byte-identical interrupted
		// candidates. Hash-then-unlink cannot guarantee an uncooperative writer's
		// later bytes survive, so this operation performs no candidate cleanup.

		if err = requireInspectedClientTree(material.CodeRoot, expectedCode); err != nil {
			return material, err
		}
		material.CodeTreeSHA256 = expectedCode
		material.Backups, err = inspectClientBackupInventory(o, old)
		if err != nil {
			return material, err
		}

		for _, backup := range material.Backups {
			if backup.Kind == "code" {
				material.CodeBackupTreeSHA256 = backup.SHA256
			} else {
				material.DesktopBackupSHA256 = backup.SHA256
			}
		}
		if err = requireInspectedClientArchive(material.DesktopPath, expectedDesktop); err != nil {
			return material, err
		}
		material.DesktopSHA256 = expectedDesktop
		return material, nil
	}
}

// These are observable identity checks, not physical custody or atomic
// publication guarantees. A mismatch leaves all bytes and prior records intact;
// the caller reports pending/partial materialization for explicit reconciliation.
func requireInspectedClientTree(path, expected string) error {
	actual, err := installer.ClientTreeDigest(path)
	if err != nil {
		return err
	}
	if actual != expected {
		return errors.New("Code packet changed after canonical inspection; preserve bytes and reconcile partial publication")
	}
	return nil
}
func requireInspectedClientArchive(path, expected string) error {
	actual, err := installer.ClientArchiveDigest(path)
	if err != nil {
		return err
	}
	if actual != expected {
		return errors.New("Desktop package changed after canonical inspection; preserve bytes and reconcile partial publication")
	}
	return nil
}
func currentGeneratorArgs(command, destination, server string, o installer.ClientOptions, s installtx.State) []string {
	args := []string{command, "--destination", destination, "--server", server, "--version", s.Release, "--product-root", o.ManagedRoot, "--user-home", o.UserHome, "--workspace", o.Request.Workspace, "--installed-manifest", s.Binding.ManifestSHA256, "--installed-client-inputs", s.Binding.ClientInputsSHA256, "--carrier-profile", installer.StableClientProfile, "--carrier-id", o.Request.CarrierID, "--carrier-version", installer.StableCarrierVersion}
	if command == "plugin" && o.Request.LifecycleContext {
		args = append(args, "--lifecycle-context")
	}
	return args
}
func writeMarketplaceAndLauncher(root string, o installer.ClientOptions) error {
	if err := os.Mkdir(filepath.Join(root, ".claude-plugin"), 0700); err != nil {
		return err
	}
	body, _ := json.MarshalIndent(map[string]any{"name": "maestro-local-" + o.Request.CarrierID, "owner": map[string]string{"name": "Maestro"}, "plugins": []any{map[string]any{"name": "maestro-product", "source": "./plugins/maestro-product"}}}, "", "  ")
	if err := scheduler.WriteNewPrivateFile(filepath.Join(root, ".claude-plugin", "marketplace.json"), append(body, '\n')); err != nil {
		return err
	}
	name, body := managementLauncher(o)
	path := filepath.Join(root, name)
	if err := scheduler.WriteNewPrivateFile(path, body); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	return os.Chmod(path, 0700)
}
func managementLauncher(o installer.ClientOptions) (string, []byte) {
	args := []string{o.Self, "manage", "--product-root", o.ManagedRoot, "--user-home", o.UserHome, "--workspace", o.Request.Workspace, "--carrier-id", o.Request.CarrierID}
	args = append(args, "--client-code-root", o.Request.CodeRoot, "--client-desktop-path", o.Request.DesktopPath, "--client-code-executable", o.Request.CodeExecutable, "--client-scope", o.Request.CodeScope)
	if o.Request.LifecycleContext {
		args = append(args, "--client-lifecycle-context")
	}
	if runtime.GOOS == "windows" {
		var q []string
		for _, s := range args {
			q = append(q, "'"+strings.ReplaceAll(s, "'", "''")+"'")
		}
		return "Manage Maestro.ps1", []byte("& " + strings.Join(q, " ") + "\nif ($LASTEXITCODE) { exit $LASTEXITCODE }\n")
	}
	var q []string
	for _, s := range args {
		q = append(q, "'"+strings.ReplaceAll(s, "'", "'\"'\"'")+"'")
	}
	return "Manage Maestro.command", []byte("#!/bin/sh\nexec " + strings.Join(q, " ") + "\n")
}
func inspectMarketplace(ctx context.Context, root string, o installer.ClientOptions, identity releasecontract.ServerIdentity, serverSHA string) error {
	if _, e := installer.ClientTreeDigest(root); e != nil {
		return e
	}
	body, e := scheduler.ReadPrivateFile(filepath.Join(root, ".claude-plugin", "marketplace.json"), 64<<10)
	if e != nil {
		return e
	}
	var marketplace struct {
		Name    string            `json:"name"`
		Owner   map[string]string `json:"owner"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	if e = releasecontract.DecodeStrictJSON(body, &marketplace); e != nil {
		return e
	}
	if marketplace.Name != "maestro-local-"+o.Request.CarrierID || len(marketplace.Plugins) != 1 || marketplace.Plugins[0].Name != "maestro-product" || marketplace.Plugins[0].Source != "./plugins/maestro-product" {
		return errors.New("foreign local marketplace; files preserved")
	}
	entries, e := os.ReadDir(root)
	if e != nil {
		return e
	}
	for _, entry := range entries {
		switch entry.Name() {
		case ".claude-plugin", "plugins", "Manage Maestro.command", "Manage Maestro.ps1":
		default:
			return errors.New("unexpected local marketplace content; preserved")
		}
	}
	plugin := filepath.Join(root, "plugins", "maestro-product")
	if _, e = inspectCurrentPacket(ctx, plugin, "plugin", o, identity, serverSHA); e != nil {
		return e
	}
	body, e = readPacketConfiguration(filepath.Join(plugin, ".mcp.json"))
	if e != nil {
		return e
	}
	var config struct {
		Servers map[string]struct {
			Type    string   `json:"type"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if e = releasecontract.DecodeStrictJSON(body, &config); e != nil {
		return e
	}
	server, found := config.Servers["maestro"]
	if len(config.Servers) != 1 || !found || server.Type != "stdio" {
		return errors.New("Code candidate server differs; preserved")
	}
	if e = checkClientLaunchBinding(server.Command, server.Args, o, runtime.GOOS); e != nil {
		return e
	}
	name, expected := managementLauncher(o)
	file, e := scheduler.OpenRegularFile(filepath.Join(root, name))
	if e != nil {
		return e
	}
	defer file.Close()
	info, e := file.Stat()
	if e != nil {
		return e
	}
	mode := os.FileMode(0700)
	if runtime.GOOS == "windows" {
		mode = 0600
	}
	if e = scheduler.ValidatePrivateFile(file, mode); e != nil {
		return e
	}
	if info.Size() > 64<<10 {
		return errors.New("Code management launcher size bound")
	}
	body, e = io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if e != nil {
		return e
	}
	if !bytes.Equal(body, expected) {
		return errors.New("Code management launcher differs from reviewed binding; preserved")
	}
	return nil
}

// Packet metadata is canonical public content inside the private carrier root;
// it has the packet's 0644 role, unlike the private 0600 marketplace record.
func readPacketConfiguration(path string) ([]byte, error) {
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err = scheduler.ValidateSingleLink(f); err != nil {
		return nil, err
	}
	before, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if before.Size() <= 0 || before.Size() > 64<<10 {
		return nil, errors.New("packet configuration bound")
	}
	body, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if int64(len(body)) != before.Size() || !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, errors.New("packet configuration changed while reading")
	}
	return body, nil
}
func inspectOwnedCode(ctx context.Context, root string, o installer.ClientOptions, old *installer.ClientRecord) error {
	if old != nil {
		digest, e := installer.ClientTreeDigest(root)
		if e == nil && digest == old.Materialization.CodeTreeSHA256 {
			return nil
		}
		if e == nil && root == old.Materialization.CodeRoot+".previous-"+o.Request.CarrierID && digest == old.Materialization.CodeBackupTreeSHA256 {
			return nil
		}
	}
	if root != o.Request.CodeRoot {
		return errors.New("unknown or changed Code backup; preserved")
	}
	name := "maestro-mcp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	// Identity is read only from a candidate whose exact server bytes are either
	// authenticated Current or the previously admitted packet's server digest.
	server := filepath.Join(root, "plugins", "maestro-product", "scripts", name)
	hash, e := installer.ClientFileDigest(server)
	if e != nil {
		return e
	}
	current, e := installtx.ReadSourceState(o.DataRoot, o.ManagedRoot)
	if e != nil {
		return e
	}
	if hash != current.Server.SHA256 && (old == nil || hash != old.Materialization.ServerSHA256) {
		return errors.New("unknown marketplace server; preserved")
	}
	identity, e := releasecontract.ReadServerIdentity(server)
	if e != nil {
		return e
	}
	return inspectMarketplace(ctx, root, o, identity, hash)
}
func preflightClientPublication(codeStage, desktopStage string, o installer.ClientOptions, reuseDesktop bool) error {
	if _, err := os.Lstat(o.Request.CodeRoot); err == nil {
		current, e := installer.ClientTreeDigest(o.Request.CodeRoot)
		if e != nil {
			return e
		}
		candidate, e := installer.ClientTreeDigest(codeStage)
		if e != nil {
			return e
		}
		if current != candidate {
			return errors.New("unavailable/client_tree_replacement_custody: preserve current and candidate packets; complete the manual client replacement step")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !reuseDesktop {
		if _, err := os.Lstat(o.Request.DesktopPath); err == nil {
			current, e := installer.ClientArchiveDigest(o.Request.DesktopPath)
			if e != nil {
				return e
			}
			candidate, e := installer.ClientArchiveDigest(desktopStage)
			if e != nil {
				return e
			}
			if current != candidate {
				return errors.New("unavailable/client_package_replacement_custody: preserve current and candidate packages; complete the manual Desktop replacement step")
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
func swapOwnedCode(ctx context.Context, stage string, o installer.ClientOptions, old *installer.ClientRecord) error {
	destination := o.Request.CodeRoot
	if _, err := os.Lstat(destination); err == nil {
		if err = inspectOwnedCode(ctx, destination, o, old); err != nil {
			return err
		}
		actual, err := installer.ClientTreeDigest(destination)
		if err != nil {
			return err
		}
		expected, err := installer.ClientTreeDigest(stage)
		if err != nil {
			return err
		}
		if actual != expected {
			return errors.New("unavailable/client_tree_replacement_custody: existing Code packet preserved")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return publishClientCandidate(stage, destination)
}

// An operator may move the exact prior slot into its immutable digest-named
// backup while completing a manual client step. This observer never moves,
// removes or overwrites anything and retains the complete prior inventory.
func inspectClientBackupInventory(o installer.ClientOptions, old *installer.ClientRecord) ([]installer.ClientBackup, error) {
	if old == nil {
		return nil, nil
	}
	backups := append([]installer.ClientBackup{}, old.Materialization.Backups...)
	seen := map[string]bool{}
	inspect := func(b installer.ClientBackup) error {
		if err := b.ValidateBinding(o.Request.CodeRoot, o.Request.DesktopPath); err != nil {
			return err
		}
		var actual string
		var err error
		switch b.Kind {
		case "code":
			actual, err = installer.ClientTreeDigest(b.Path)
		case "desktop":
			actual, err = installer.ClientArchiveDigest(b.Path)
		default:
			return errors.New("unknown client backup role")
		}
		if err != nil || actual != b.SHA256 {
			return errors.New("unknown or changed immutable client backup; preserved")
		}
		seen[b.Path] = true
		return nil
	}
	for _, b := range backups {
		if err := inspect(b); err != nil {
			return nil, err
		}
	}
	for _, b := range []installer.ClientBackup{
		{Kind: "code", Path: o.Request.CodeRoot + ".previous-" + old.Materialization.CodeTreeSHA256, SHA256: old.Materialization.CodeTreeSHA256},
		{Kind: "desktop", Path: o.Request.DesktopPath + ".previous-" + old.Materialization.DesktopSHA256 + ".mcpb", SHA256: old.Materialization.DesktopSHA256},
	} {
		if seen[b.Path] {
			continue
		}
		if _, err := os.Lstat(b.Path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		if len(backups) >= 64 {
			return nil, errors.New("client backup inventory capacity exhausted; preserve all backups")
		}
		if err := inspect(b); err != nil {
			return nil, err
		}
		backups = append(backups, b)
	}
	return backups, nil
}
func inspectDesktopBackup(o installer.ClientOptions, old *installer.ClientRecord) (string, error) {
	backups, err := inspectClientBackupInventory(o, old)
	if err != nil {
		return "", err
	}
	for i := len(backups) - 1; i >= 0; i-- {
		if backups[i].Kind == "desktop" {
			return backups[i].SHA256, nil
		}
	}
	return "", nil
}

// The full packet inspector proves Current server/resources/profile. Its
// returned native hash becomes expected authority only after this exact local
// binding check, including when a candidate survives an interrupted process.
func inspectDesktopCandidateBinding(path string, o installer.ClientOptions, targetOS, nativeContract string) error {
	file, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() <= 0 || info.Size() > 128<<20 {
		return errors.New("Desktop candidate size bound")
	}
	archive, err := zip.NewReader(file, info.Size())
	if err != nil {
		return err
	}
	var body []byte
	for _, entry := range archive.File {
		if entry.Name != "manifest.json" {
			continue
		}
		if body != nil || entry.UncompressedSize64 > 64<<10 {
			return errors.New("Desktop candidate native manifest bound")
		}
		reader, e := entry.Open()
		if e != nil {
			return e
		}
		body, err = io.ReadAll(io.LimitReader(reader, (64<<10)+1))
		closeErr := reader.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if len(body) == 0 || len(body) > 64<<10 || fmt.Sprintf("%x", sha256.Sum256(body)) != nativeContract {
		return errors.New("Desktop candidate native contract changed; preserved")
	}
	var manifest struct {
		Server struct {
			Configuration json.RawMessage `json:"mcp_config"`
		} `json:"server"`
	}
	if err = json.Unmarshal(body, &manifest); err != nil {
		return err
	}
	// Closed native configuration: no extra env, type, args or other fields.
	var config struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if err = releasecontract.DecodeStrictJSON(manifest.Server.Configuration, &config); err != nil {
		return err
	}
	return checkClientLaunchBinding(config.Command, config.Args, o, targetOS)
}
func checkClientLaunchBinding(command string, args []string, o installer.ClientOptions, targetOS string) error {
	launcher := "maestro-install"
	if targetOS == "windows" {
		launcher += ".exe"
	}
	expected := []string{"stable-launch", "--product-root", o.ManagedRoot, "--user-home", o.UserHome, "--workspace", o.Request.Workspace, "--carrier-id", o.Request.CarrierID}
	if command != filepath.Join(o.ManagedRoot, launcher) || len(args) != len(expected) {
		return errors.New("client candidate launch binding differs; preserved")
	}
	for i, arg := range expected {
		if args[i] != arg {
			return errors.New("client candidate launch binding differs; preserved")
		}
	}
	return nil
}
func inspectCurrentDesktopContract(ctx context.Context, path string, o installer.ClientOptions, identity releasecontract.ServerIdentity, serverSHA, expectedNative string) error {
	status, err := inspectCurrentPacket(ctx, path, "mcpb", o, identity, serverSHA)
	native := status.NativeContractSHA256
	if err != nil {
		return err
	}
	if native != expectedNative {
		return errors.New("Desktop native contract differs from reviewed current binding; preserved")
	}
	return nil
}
func swapOwnedDesktop(ctx context.Context, stage string, o installer.ClientOptions, old *installer.ClientRecord, identity releasecontract.ServerIdentity, nativeContract string) error {
	destination := o.Request.DesktopPath
	if _, err := os.Lstat(destination); err == nil {
		if err = inspectCurrentDesktopContract(ctx, destination, o, identity, identityServerSHA(o), nativeContract); err != nil {
			return err
		}
		actual, err := installer.ClientArchiveDigest(destination)
		if err != nil {
			return err
		}
		expected, err := installer.ClientArchiveDigest(stage)
		if err != nil {
			return err
		}
		if actual != expected {
			return errors.New("unavailable/client_package_replacement_custody: existing Desktop package preserved")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return publishClientCandidate(stage, destination)
}
func identityServerSHA(o installer.ClientOptions) string {
	state, err := installtx.ReadSourceState(o.DataRoot, o.ManagedRoot)
	if err != nil {
		return ""
	}
	return state.Server.SHA256
}

func RegisterCode(ctx context.Context, o installer.ClientOptions, run ClientCommand, digest string, confirmed bool) (installer.ClientRecord, error) {
	status, err := installer.ReadClientsSeeded(ctx, o)
	if err != nil {
		return installer.ClientRecord{}, err
	}
	if status.State != "current_materialization" || status.Record == nil {
		return installer.ClientRecord{}, errors.New("current client materialization required")
	}
	record := *status.Record
	return installer.RegisterCodeSeeded(ctx, o, installer.ClientRecordDigest(record), func(current installer.ClientRecord) (installer.CodeObservation, error) {
		return ApplyCodeRegistration(ctx, current, run, digest, confirmed)
	})
}
func ConfirmDesktop(ctx context.Context, o installer.ClientOptions, packageSHA, contractSHA, digest string, confirmed bool) (installer.ClientRecord, error) {
	status, err := installer.ReadClientsSeeded(ctx, o)
	if err != nil {
		return installer.ClientRecord{}, err
	}
	if status.Record == nil || status.State != "current_materialization" || !confirmed {
		return installer.ClientRecord{}, errors.New("explicit current Desktop package confirmation required")
	}
	r := *status.Record
	if digest != installer.ClientRecordDigest(r) {
		return r, errors.New("Desktop confirmation changed")
	}
	d := installer.DesktopObservation{Handoff: "bundle_ready", InstallConfirmation: "user_confirmed_install", Connection: "pending", FunctionalCheck: "pending", PackageSHA256: packageSHA, NativeContractSHA256: contractSHA, ManifestSHA256: r.Materialization.ManifestSHA256, EvidenceClass: "operator_attested"}
	return installer.ObserveClientsSeeded(ctx, o, digest, nil, &d)
}
