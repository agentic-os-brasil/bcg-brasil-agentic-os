package installer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// ClientManagementLauncher is the sole canonical participant console recipe.
// update selects one closed foreground intent, retaining every installer review.
func ClientManagementLauncher(o ClientOptions, update bool) (string, []byte) {
	return clientManagementLauncherForOS(o, update, runtime.GOOS)
}

func clientManagementLauncherForOS(o ClientOptions, update bool, targetOS string) (string, []byte) {
	args := []string{o.Self, "manage", "--product-root", o.ManagedRoot, "--user-home", o.UserHome, "--workspace", o.Request.Workspace, "--carrier-id", o.Request.CarrierID, "--client-code-root", o.Request.CodeRoot, "--client-desktop-path", o.Request.DesktopPath, "--client-code-executable", o.Request.CodeExecutable, "--client-scope", o.Request.CodeScope}
	if o.Request.CodeConfigurationDirectory != "" {
		args = append(args, "--client-code-config", o.Request.CodeConfigurationDirectory)
	}
	if o.Request.LifecycleContext {
		args = append(args, "--client-lifecycle-context")
	}
	name := "Manage Maestro"
	if update {
		name = "Update Maestro"
		args = append(args, "--management-intent", "update")
	}
	var quoted []string
	if targetOS == "windows" {
		for _, arg := range args {
			quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "''")+"'")
		}
		body := "& " + strings.Join(quoted, " ") + "\nif ($LASTEXITCODE) { exit $LASTEXITCODE }\n"
		if profile := o.Request.CodeConfigurationDirectory; profile != "" {
			value := "'" + strings.ReplaceAll(profile, "'", "''") + "'"
			body = "$maestroPreviousConfig = $env:CLAUDE_CONFIG_DIR\ntry {\n$env:CLAUDE_CONFIG_DIR = " + value + "\n$global:LASTEXITCODE = $null\n& " + strings.Join(quoted, " ") + "\n$maestroExit = $global:LASTEXITCODE\nif ($null -eq $maestroExit) { throw \"Maestro executable did not return a child exit status\" }\n} finally { $env:CLAUDE_CONFIG_DIR = $maestroPreviousConfig }\nif ($maestroExit) { exit $maestroExit }\n"
		}
		return name + ".ps1", []byte(body)
	}
	for _, arg := range args {
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "'\"'\"'")+"'")
	}
	prefix := ""
	if profile := o.Request.CodeConfigurationDirectory; profile != "" {
		prefix = "/usr/bin/env '" + strings.ReplaceAll("CLAUDE_CONFIG_DIR="+profile, "'", "'\"'\"'") + "' "
	}
	return name + ".command", []byte("#!/bin/sh\nexec " + prefix + strings.Join(quoted, " ") + "\n")
}

type ClientManagementHandoff struct {
	CarrierID      string `json:"carrier_id"`
	WorkspaceID    string `json:"workspace_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
	Launcher       string `json:"launcher"`
	State          string `json:"state"`
	Activation     string `json:"activation"`
}

// PrepareClientManagementHandoff opens nothing and issues no approval. Only
// trusted startup composition supplies the fixed installed roots and carrier.
func PrepareClientManagementHandoff(ctx context.Context, o ClientOptions) (ClientManagementHandoff, error) {
	if !carrierIDPattern.MatchString(o.Request.CarrierID) {
		return ClientManagementHandoff{}, errors.New("exact unambiguous fixed management carrier required")
	}
	if _, err := admitOperationSeed(ctx, o.SeededOperationOptions); err != nil {
		return ClientManagementHandoff{}, err
	}
	var out ClientManagementHandoff
	err := installtx.GuardQualifiedCurrent(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now(), func() error {
		r, _, err := readClientRecord(o.SeededOperationOptions, o.Request.CarrierID)
		if err != nil {
			return err
		}
		if r.Materialization.Selection == nil || r.Binding.Workspace != o.Request.Workspace {
			return errors.New("qualified management carrier/workspace context unavailable")
		}
		bound := o
		bound.Request = ClientRequestFromRecord(r)
		status, err := ReadClientsSeeded(ctx, bound)
		if err != nil || status.State != "current_materialization" {
			return errors.New("current management binding unavailable")
		}
		// The published launcher was generated with its actual manifest slot paths.
		generation := bound
		generation.Request.CodeRoot = r.Materialization.CodeRoot
		generation.Request.DesktopPath = r.Materialization.DesktopPath
		name, expected := ClientManagementLauncher(generation, true)
		path := filepath.Join(r.Materialization.CodeRoot, name)
		if err := verifyManagementFile(path, expected, runtime.GOOS != "windows"); err != nil {
			return err
		}
		if runtime.GOOS == "windows" {
			path = filepath.Join(r.Materialization.CodeRoot, "Update Maestro.cmd")
			if err := verifyManagementFile(path, ClientWindowsManagementEntry(), false); err != nil {
				return err
			}
		}
		if err := recheckExperimentalOperation(o.SeededOperationOptions); err != nil {
			return err
		}
		out = ClientManagementHandoff{CarrierID: r.CarrierID, WorkspaceID: r.Binding.WorkspaceID, ManifestSHA256: r.Materialization.ManifestSHA256, Launcher: path, State: "handoff_prepared", Activation: "not_requested_or_approved"}
		return nil
	})
	if err == nil {
		err = recheckExperimentalOperation(o.SeededOperationOptions)
	}
	return out, err
}

func ClientWindowsManagementEntry() []byte {
	return []byte("@echo off\r\npowershell.exe -NoProfile -File \"%~dp0Update Maestro.ps1\"\r\nexit /b %errorlevel%\r\n")
}

func verifyManagementFile(path string, expected []byte, executable bool) error {
	file, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return err
	}
	defer file.Close()
	mode := os.FileMode(0600)
	if executable {
		mode = 0700
	}
	if err := scheduler.ValidatePrivateFile(file, mode); err != nil {
		return err
	}
	body, err := io.ReadAll(io.LimitReader(file, 64<<10+1))
	if err != nil {
		return err
	}
	if !bytes.Equal(body, expected) {
		return errors.New("fixed management launcher changed; preserved")
	}
	return nil
}
