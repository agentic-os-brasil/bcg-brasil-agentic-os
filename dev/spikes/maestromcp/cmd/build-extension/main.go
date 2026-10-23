// build-extension packages a local synthetic Desktop experiment, never a release.
package main

import (
	"archive/zip"
	"bytes"
	"debug/macho"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/dev/spikes/maestromcp/spike"
)

func run(args []string, out, diagnostic io.Writer) int {
	flags := flag.NewFlagSet("build-extension", flag.ContinueOnError)
	flags.SetOutput(diagnostic)
	destination := flags.String("destination", "", "new .mcpb file (existing parent required)")
	server := flags.String("server", "", "absolute native macOS server binary to copy")
	state := flags.String("state-root", "", "absolute existing synthetic state root, outside artifact")
	workspace := flags.String("workspace", "", "fixed server workspace: case-a or case-b")
	version := flags.String("version", "0.0.1", "spike revision: 0.0.1 or 0.0.2")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(diagnostic, "build-extension: unexpected positional arguments")
		return 2
	}
	if err := build(*destination, *server, *state, *workspace, *version); err != nil {
		fmt.Fprintln(diagnostic, "build-extension:", err)
		return 2
	}
	absolute, _ := filepath.Abs(*destination)
	fmt.Fprintln(out, absolute)
	return 0
}

func build(destination, server, state, workspace, version string) error {
	if destination == "" || !filepath.IsAbs(server) || !filepath.IsAbs(state) {
		return errors.New("destination and absolute server/state-root paths are required")
	}
	if filepath.Ext(destination) != ".mcpb" {
		return errors.New("destination must be a new .mcpb file")
	}
	if version != "0.0.1" && version != "0.0.2" {
		return errors.New("version must be 0.0.1 or 0.0.2")
	}
	// Desktop substitutes MCPB variables. A fixed owner-selected root must not
	// accidentally become a host-expanded variable expression.
	if strings.Contains(state, "${") {
		return errors.New("state-root must not contain MCPB variable expressions")
	}
	if _, err := spike.Open(state, workspace); err != nil {
		return fmt.Errorf("existing synthetic fixture required: %w", err)
	}
	destination, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(destination))
	if err != nil {
		return fmt.Errorf("destination parent must exist: %w", err)
	}
	parentInfo, err := os.Stat(resolvedParent)
	if err != nil || !parentInfo.IsDir() {
		return errors.New("destination parent must be an existing directory")
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return errors.New("current user's absolute home directory unavailable")
	}
	developer, err := filepath.EvalSymlinks(filepath.Join(home, "Developer"))
	if err != nil || !contains(developer, resolvedParent) {
		return errors.New("extension destination must be physically under the current user's Developer directory")
	}
	resolvedState, err := filepath.EvalSymlinks(state)
	if err != nil {
		return err
	}
	resolvedDestination := filepath.Join(resolvedParent, filepath.Base(destination))
	if contains(resolvedState, resolvedDestination) || contains(resolvedDestination, resolvedState) {
		return errors.New("extension artifact and state-root must not contain one another")
	}
	info, err := os.Lstat(server)
	if err != nil {
		return fmt.Errorf("server binary: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("server must be a regular native binary, not a symlink")
	}
	// The native binary is an explicitly selected build input; no directory is
	// copied and no profile, credentials, hooks or owner state enter this ZIP.
	binary, err := os.Open(server)
	if err != nil {
		return err
	}
	defer binary.Close()
	// This bundle advertises macOS only. Reject an accidentally supplied ELF,
	// PE or script instead of mislabeling a host-native non-macOS executable.
	// Header parsing does not attest signing, notarization or host acceptance.
	native, err := macho.NewFile(binary)
	if err != nil || native.Type != macho.TypeExec || (native.Cpu != macho.CpuArm64 && native.Cpu != macho.CpuAmd64) {
		return errors.New("server must be a thin macOS ARM64 or AMD64 Mach-O executable")
	}
	manifest, err := json.MarshalIndent(map[string]any{
		"manifest_version": "0.3",
		"name":             "maestro-architecture-spike",
		"version":          version,
		"description":      "Non-production local synthetic Maestro MCP and filesystem experiment",
		"author":           map[string]string{"name": "Agentic OS Brasil"},
		"server": map[string]any{
			"type":        "binary",
			"entry_point": "server/maestro-mcp-spike",
			"mcp_config": map[string]any{
				"command": "${__dirname}/server/maestro-mcp-spike",
				"args":    []string{"serve", "--root", state, "--workspace", workspace},
			},
		},
		"compatibility": map[string]any{"platforms": []string{"darwin"}},
	}, "", "  ")
	if err != nil {
		return err
	}
	artifact, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("destination must be new; nothing overwritten: %w", err)
	}
	// A failed build keeps its newly created artifact for inspection. Existing
	// artifacts and external state are never removed as part of packaging.
	archive := zip.NewWriter(artifact)
	writeErr := addEntry(archive, "manifest.json", 0600, bytes.NewReader(append(manifest, '\n')))
	if writeErr == nil {
		writeErr = addEntry(archive, "server/maestro-mcp-spike", 0700, binary)
	}
	return errors.Join(writeErr, archive.Close(), artifact.Close())
}

func addEntry(archive *zip.Writer, name string, mode os.FileMode, input io.Reader) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(mode)
	output, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, input)
	return err
}

func contains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
