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
		stage := o.Request.CodeRoot + ".candidate-" + o.Request.CarrierID
		desktopStage := o.Request.DesktopPath + ".candidate-" + o.Request.CarrierID + ".mcpb"
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
		if err = inspectMarketplace(stage, o, identity, s.Server.SHA256); err != nil {
			return installer.ClientMaterialization{}, err
		}
		if _, e := os.Lstat(desktopStage); errors.Is(e, os.ErrNotExist) {
			if _, err = clientCommand(ctx, server, currentGeneratorArgs("mcpb", desktopStage, server, o, s), b.Workspace); err != nil {
				return installer.ClientMaterialization{}, fmt.Errorf("actual current MCPB generator: %w", err)
			}
		} else if e != nil {
			return installer.ClientMaterialization{}, e
		}
		nativeContract, err := packaging.InspectStableExtension(desktopStage, expected, identity, s.Server.SHA256)
		if err != nil {
			return installer.ClientMaterialization{}, err
		}
		if err = inspectDesktopCandidateBinding(desktopStage, o, s.TargetOS, nativeContract); err != nil {
			return installer.ClientMaterialization{}, err
		}
		material := installer.ClientMaterialization{Release: s.Release, ManifestSHA256: s.Binding.ManifestSHA256, ClientInputsSHA256: s.Binding.ClientInputsSHA256, ServerSHA256: s.Server.SHA256, EmbeddedInputsSHA256: s.Server.EmbeddedInputsSHA256, BundleSHA256: s.Binding.BundleSHA256, CodeRoot: o.Request.CodeRoot, DesktopPath: o.Request.DesktopPath, DesktopNativeContractSHA256: nativeContract, BootstrapServerSHA256: s.Server.SHA256, BootstrapEmbeddedInputsSHA256: s.Server.EmbeddedInputsSHA256}
		// First publication may encounter an already-swapped destination. Admit
		// its exact reviewed native contract before any Code or Desktop swap.
		if old == nil {
			if _, e := os.Lstat(o.Request.DesktopPath); e == nil {
				if e = inspectCurrentDesktopContract(o.Request.DesktopPath, expected, identity, s.Server.SHA256, nativeContract); e != nil {
					return material, e
				}
			} else if !errors.Is(e, os.ErrNotExist) {
				return material, e
			}
		}
		// A reviewed recovery may finish an authenticated current replacement
		// whose swap completed before its finite current record was published.
		adoptCurrent := false
		if old != nil && o.Request.Reconcile {
			sha, e := installer.ClientFileDigest(o.Request.DesktopPath)
			if e == nil && sha != old.Materialization.DesktopSHA256 {
				if e = inspectCurrentDesktopContract(o.Request.DesktopPath, expected, identity, s.Server.SHA256, nativeContract); e != nil {
					return material, e
				}
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
			material.BootstrapServerSHA256 = m.BootstrapServerSHA256
			material.BootstrapEmbeddedInputsSHA256 = m.BootstrapEmbeddedInputsSHA256
		}
		if err = swapOwnedCode(stage, o, old); err != nil {
			return material, err
		}
		if reuse {
			if err = os.Remove(desktopStage); err != nil {
				return material, err
			}
		} else if adoptCurrent {
			if err = os.Remove(desktopStage); err != nil {
				return material, err
			}
		} else if err = swapOwnedDesktop(desktopStage, o, old, identity, nativeContract); err != nil {
			return material, err
		}
		material.CodeTreeSHA256, err = installer.ClientTreeDigest(material.CodeRoot)
		if err != nil {
			return material, err
		}
		backup := material.CodeRoot + ".previous-" + o.Request.CarrierID
		if _, e := os.Lstat(backup); e == nil {
			material.CodeBackupTreeSHA256, err = installer.ClientTreeDigest(backup)
			if err != nil {
				return material, err
			}
		} else if !errors.Is(e, os.ErrNotExist) {
			return material, e
		}
		material.DesktopBackupSHA256, err = inspectDesktopBackup(o, old)
		if err != nil {
			return material, err
		}
		material.DesktopSHA256, err = installer.ClientFileDigest(material.DesktopPath)
		return material, err
	}
}
func currentGeneratorArgs(command, destination, server string, o installer.ClientOptions, s installtx.State) []string {
	return []string{command, "--destination", destination, "--server", server, "--version", s.Release, "--product-root", o.ManagedRoot, "--user-home", o.UserHome, "--workspace", o.Request.Workspace, "--installed-manifest", s.Binding.ManifestSHA256, "--installed-client-inputs", s.Binding.ClientInputsSHA256, "--carrier-profile", installer.StableClientProfile, "--carrier-id", o.Request.CarrierID, "--carrier-version", installer.StableCarrierVersion}
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
func inspectMarketplace(root string, o installer.ClientOptions, identity releasecontract.ServerIdentity, serverSHA string) error {
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
	if e = packaging.InspectStablePlugin(plugin, stableCarrier(o.Request.CarrierID), identity, serverSHA); e != nil {
		return e
	}
	body, e = scheduler.ReadPrivateFile(filepath.Join(plugin, ".mcp.json"), 64<<10)
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
	if e = validateClientPrivateFile(info, mode); e != nil {
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
func inspectOwnedCode(root string, o installer.ClientOptions, old *installer.ClientRecord) error {
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
	return inspectMarketplace(root, o, identity, hash)
}
func swapOwnedCode(stage string, o installer.ClientOptions, old *installer.ClientRecord) error {
	destination := o.Request.CodeRoot
	backup := destination + ".previous-" + o.Request.CarrierID
	if _, e := os.Lstat(destination); e == nil {
		if e = inspectOwnedCode(destination, o, old); e != nil {
			return e
		}
		if _, e = os.Lstat(backup); e == nil {
			if e = inspectOwnedCode(backup, o, old); e != nil {
				return e
			}
			if e = os.RemoveAll(backup); e != nil {
				return e
			}
		} else if !errors.Is(e, os.ErrNotExist) {
			return e
		}
		if e = os.Rename(destination, backup); e != nil {
			return e
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	return os.Rename(stage, destination)
}

// Only the fixed backup's exact previously recorded bytes or the exact
// pre-record interrupted parent are owned. Aliased/unknown files stay intact.
func inspectDesktopBackup(o installer.ClientOptions, old *installer.ClientRecord) (string, error) {
	path := o.Request.DesktopPath + ".previous-" + o.Request.CarrierID + ".mcpb"
	if _, e := os.Lstat(path); errors.Is(e, os.ErrNotExist) {
		return "", nil
	} else if e != nil {
		return "", e
	}
	file, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() <= 0 || info.Size() > 128<<20 {
		return "", errors.New("Desktop backup size bound")
	}
	if err = validateClientPrivateFile(info, 0600); err != nil {
		return "", err
	}
	h := sha256.New()
	size, err := io.Copy(h, io.LimitReader(file, (128<<20)+1))
	if err != nil {
		return "", err
	}
	if size == 0 || size > 128<<20 {
		return "", errors.New("Desktop backup size bound")
	}
	sha := fmt.Sprintf("%x", h.Sum(nil))
	if old == nil || sha != old.Materialization.DesktopSHA256 && sha != old.Materialization.DesktopBackupSHA256 {
		return "", errors.New("unknown previous Desktop package preserved")
	}
	return sha, nil
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
func inspectCurrentDesktopContract(path string, carrier packaging.StableCarrier, identity releasecontract.ServerIdentity, serverSHA, expectedNative string) error {
	native, err := packaging.InspectStableExtension(path, carrier, identity, serverSHA)
	if err != nil {
		return err
	}
	if native != expectedNative {
		return errors.New("Desktop native contract differs from reviewed current binding; preserved")
	}
	return nil
}
func swapOwnedDesktop(stage string, o installer.ClientOptions, old *installer.ClientRecord, identity releasecontract.ServerIdentity, nativeContract string) error {
	destination := o.Request.DesktopPath
	backup := destination + ".previous-" + o.Request.CarrierID + ".mcpb"
	knownBackup, err := inspectDesktopBackup(o, old)
	if err != nil {
		return err
	}
	if _, e := os.Lstat(destination); e == nil {
		if old != nil {
			m := old.Materialization
			if e = packaging.InspectReusedStableExtension(destination, stableCarrier(o.Request.CarrierID), m.DesktopSHA256, m.BootstrapServerSHA256, m.BootstrapEmbeddedInputsSHA256, m.DesktopNativeContractSHA256); e != nil {
				return e
			}
		} else {
			current, e := installtx.ReadSourceState(o.DataRoot, o.ManagedRoot)
			if e != nil {
				return e
			}
			if e = inspectCurrentDesktopContract(destination, stableCarrier(o.Request.CarrierID), identity, current.Server.SHA256, nativeContract); e != nil {
				return e
			}
			// An interrupted first publication already has the fully verified
			// current destination; finish without inventing an unrecorded backup.
			return os.Remove(stage)
		}
		if knownBackup != "" {
			if e = os.Remove(backup); e != nil {
				return e
			}
		}
		if e = os.Rename(destination, backup); e != nil {
			return e
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	return os.Rename(stage, destination)
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
