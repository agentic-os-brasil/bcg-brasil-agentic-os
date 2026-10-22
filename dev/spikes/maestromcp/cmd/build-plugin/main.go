// build-plugin packages a disposable MCP architecture test, never a release.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	assets "github.com/agentic-os-brasil/bcg-brasil-agentic-os/dev/spikes/maestromcp/plugin-assets"
)

func run(args []string, out, diagnostic io.Writer) int {
	flags := flag.NewFlagSet("build-plugin", flag.ContinueOnError)
	flags.SetOutput(diagnostic)
	destination := flags.String("destination", "", "new plugin directory (existing parent required)")
	server := flags.String("server", "", "native server binary to copy")
	state := flags.String("state-root", "", "absolute existing synthetic state root, outside plugin")
	workspace := flags.String("workspace", "", "fixed server workspace: case-a or case-b")
	version := flags.String("version", "0.0.1", "spike revision: 0.0.1 or 0.0.2")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(diagnostic, "build-plugin: unexpected positional arguments")
		return 2
	}
	if err := build(*destination, *server, *state, *workspace, *version); err != nil {
		fmt.Fprintln(diagnostic, "build-plugin:", err)
		return 2
	}
	absolute, _ := filepath.Abs(*destination)
	fmt.Fprintln(out, absolute)
	return 0
}

func build(destination, server, state, workspace, version string) error {
	if destination == "" || server == "" || !filepath.IsAbs(state) {
		return errors.New("destination, server and an absolute state-root are required")
	}
	if workspace != "case-a" && workspace != "case-b" {
		return errors.New("workspace must be case-a or case-b")
	}
	if version != "0.0.1" && version != "0.0.2" {
		return errors.New("version must be 0.0.1 or 0.0.2")
	}
	destination, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	stateInfo, err := os.Stat(state)
	if err != nil || !stateInfo.IsDir() {
		return errors.New("state-root must be an existing directory; builder never creates state")
	}
	resolvedState, err := filepath.EvalSymlinks(state)
	if err != nil {
		return err
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(destination))
	if err != nil {
		return fmt.Errorf("destination parent must exist: %w", err)
	}
	resolvedDestination := filepath.Join(resolvedParent, filepath.Base(destination))
	if contains(resolvedState, resolvedDestination) || contains(resolvedDestination, resolvedState) {
		return errors.New("plugin and state-root must not contain one another")
	}
	info, err := os.Lstat(server)
	if err != nil {
		return fmt.Errorf("server binary: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("server must be a regular native binary, not a symlink")
	}
	if err := os.Mkdir(destination, 0700); err != nil {
		return fmt.Errorf("destination must be new; nothing overwritten: %w", err)
	}
	// A failed build retains its new output for inspection, never deletes state.
	for _, dir := range []string{".claude-plugin", "scripts", "hooks"} {
		if err := os.Mkdir(filepath.Join(destination, dir), 0700); err != nil {
			return err
		}
	}
	binaryName := "maestro-mcp-spike"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	if err := copyBinary(server, filepath.Join(destination, "scripts", binaryName)); err != nil {
		return err
	}
	command := "${CLAUDE_PLUGIN_ROOT}/scripts/" + binaryName
	manifests := map[string]any{
		".claude-plugin/plugin.json": map[string]any{
			"name": "maestro-architecture-spike", "version": version,
			"description": "Non-production synthetic MCP, plugin and filesystem experiment",
		},
		".mcp.json": map[string]any{"mcpServers": map[string]any{"maestro": map[string]any{
			"type": "stdio", "command": command,
			"args": []string{"serve", "--root", state, "--workspace", workspace},
		}}},
		"hooks/hooks.json": map[string]any{"hooks": map[string]any{"SessionStart": []any{map[string]any{
			"hooks": []any{map[string]any{"type": "command", "command": command, "args": []string{"orient", "--revision", version}}},
		}}}},
	}
	for name, body := range manifests {
		encoded, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(destination, filepath.FromSlash(name)), append(encoded, '\n'), 0600); err != nil {
			return err
		}
	}
	return fs.WalkDir(assets.Files, "skills", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		target := filepath.Join(destination, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.Mkdir(target, 0700)
		}
		body, err := assets.Files.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, []byte(strings.ReplaceAll(string(body), "{{VERSION}}", version)), 0600)
	})
}

func contains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func copyBinary(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	return errors.Join(copyErr, out.Close())
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
