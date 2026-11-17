package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func commandSourceFiles(o installer.ClientOptions, platform string) (string, map[string][]byte, string, error) {
	root := o.Request.CodeRoot + ".provider-" + o.Request.CarrierID
	args := []string{o.Self, "client-plugin-path", "--product-root", o.ManagedRoot, "--user-home", o.UserHome, "--workspace", o.Request.Workspace, "--carrier-id", o.Request.CarrierID}
	var quoted []string
	name := "Maestro plugin path.command"
	for _, arg := range args {
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "'\"'\"'")+"'")
	}
	script := []byte("#!/bin/sh\nexec " + strings.Join(quoted, " ") + "\n")
	command := "'" + strings.ReplaceAll(filepath.Join(root, name), "'", "'\"'\"'") + "'"
	if platform == "windows" {
		name = "Maestro plugin path.ps1"
		quoted = nil
		for _, arg := range args {
			quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "''")+"'")
		}
		script = []byte("$ErrorActionPreference = 'Stop'\n& " + strings.Join(quoted, " ") + "\nexit $LASTEXITCODE\n")
		path := filepath.Join(root, name)
		if strings.ContainsAny(path, "\"%!?\r\n") {
			return "", nil, "", errors.New("Code command source cannot represent this Windows launcher path")
		}
		command = "powershell.exe -NoProfile -NonInteractive -File \"" + path + "\""
	}
	if len(command) == 0 || len(command) > 500 || strings.Contains(command, "    ") {
		return "", nil, "", errors.New("Code command source exceeds native command bound")
	}
	for _, ch := range command {
		if ch < 32 || ch > 126 {
			return "", nil, "", errors.New("Code command source requires a printable ASCII launcher path")
		}
	}
	manifest, err := json.MarshalIndent(map[string]any{"name": "maestro-local-" + o.Request.CarrierID, "owner": map[string]string{"name": "Maestro"}, "plugins": []any{map[string]any{"name": "maestro-product", "source": map[string]any{"source": "command", "command": command, "timeout": 30, "mode": "copy"}}}}, "", "  ")
	if err != nil {
		return "", nil, "", err
	}
	return root, map[string][]byte{".claude-plugin/marketplace.json": append(manifest, '\n'), name: script}, name, nil
}

func ensureCommandMarketplace(o installer.ClientOptions) (string, error) {
	root, files, script, err := commandSourceFiles(o, runtime.GOOS)
	if err != nil {
		return "", err
	}
	if err := scheduler.ValidatePrivateDirectory(filepath.Dir(root)); err != nil {
		return "", err
	}
	created := false
	if err := os.Mkdir(root, 0700); err == nil {
		created = true
	} else if !errors.Is(err, os.ErrExist) {
		return "", err
	}
	if !created {
		if err := scheduler.ValidatePrivateDirectory(root); err != nil {
			return "", err
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			return "", err
		}
		if len(entries) != 2 {
			return "", errors.New("occupied Code provider preserved; no repair of unknown or partial trees")
		}
		meta, err := os.ReadDir(filepath.Join(root, ".claude-plugin"))
		if err != nil || len(meta) != 1 || meta[0].Name() != "marketplace.json" {
			return "", errors.New("occupied Code provider metadata preserved")
		}
	}
	for _, dir := range []string{root, filepath.Join(root, ".claude-plugin")} {
		if err := os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", err
		}
		if err := scheduler.ValidatePrivateDirectory(dir); err != nil {
			return "", err
		}
	}
	for name, expected := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		mode := os.FileMode(0600)
		if name == script && runtime.GOOS != "windows" {
			mode = 0700
		}
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			if !created {
				return "", errors.New("incomplete Code provider preserved; no inferred repair authority")
			}
			if err := scheduler.WriteNewPrivateFile(path, expected); err != nil {
				return "", err
			}
			if mode == 0700 {
				if err := os.Chmod(path, mode); err != nil {
					return "", err
				}
			}
		} else if err != nil {
			return "", err
		}
		file, err := scheduler.OpenRegularFile(path)
		if err != nil {
			return "", err
		}
		if err := scheduler.ValidatePrivateFile(file, mode); err != nil {
			file.Close()
			return "", err
		}
		body, readErr := io.ReadAll(io.LimitReader(file, 64<<10+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			return "", errors.Join(readErr, closeErr)
		}
		if !bytes.Equal(body, expected) {
			return "", errors.New("fixed Code provider changed; preserved unchanged")
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	if len(entries) != 2 {
		return "", errors.New("unexpected Code provider entries; preserved")
	}
	meta, err := os.ReadDir(filepath.Join(root, ".claude-plugin"))
	if err != nil || len(meta) != 1 || meta[0].Name() != "marketplace.json" {
		return "", errors.New("unexpected Code provider metadata; preserved")
	}
	return root, nil
}
