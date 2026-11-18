//go:build maestro_experimental_installed_seed

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/pilotdocument"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userlevel"
)

type pilotDocumentPreparation struct {
	SchemaVersion       int    `json:"schema_version"`
	State               string `json:"state"`
	Source              string `json:"source"`
	SourceSHA256        string `json:"source_sha256"`
	WorkspaceID         string `json:"workspace_id,omitempty"`
	Conversion          string `json:"conversion"`
	NativeQualification string `json:"native_qualification"`
}

// Separate explicit participant action; the checker never calls this owner.
// There is no caller-selected source, filename, destination, content or repair.
func runInstalledPilotPrepareDocument(ctx context.Context, args []string, output io.Writer) error {
	result := pilotDocumentPreparation{SchemaVersion: 1, State: "fixture_not_prepared", Source: pilotdocument.Name, SourceSHA256: pilotdocument.SHA256, Conversion: "not_tested", NativeQualification: "not_tested"}
	err := func() error {
		if err := userlevel.EnsureNotElevated(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(args) != 14 {
			return errors.New("exact installed document preparation selection required")
		}
		for _, arg := range args {
			if strings.TrimLeft(strings.SplitN(arg, "=", 2)[0], "-") == "check-only" {
				return errors.New("read-only checks cannot prepare a workspace document")
			}
		}
		selection, err := parseInstalledPilotSelection(args)
		if err != nil {
			return err
		}
		fixed, err := installer.LoadExperimentalInstalledContext(selection.ProductRoot, AuthorityRegistrySHA256, time.Now)
		if err != nil {
			return errors.New("installed document preparation admission unavailable")
		}
		if fixed.UserHome != selection.UserHome || fixed.Version != selection.InstallerVersion || fixed.ExperimentalSeed.Identity().AdmissionSHA256 != selection.AdmissionSHA256 {
			return errors.New("installed document preparation binding differs")
		}
		state, err := installtx.ValidateQualifiedLaunch(fixed.ManagedRoot, fixed.DataRoot, installtx.ReleaseBinding{}, time.Now())
		if err != nil {
			return err
		}
		actual, err := processidentity.Current()
		if err != nil || !installedCurrentImageMatches(actual.Path, actual.SHA256, Version, selection.ProductRoot, state) {
			return errors.New("document preparation must use the actual current installed backend")
		}
		clients := installer.ClientOptions{SeededOperationOptions: fixed, Request: installer.ClientRequest{CarrierID: selection.CarrierID, Workspace: selection.Workspace}}
		err = installtx.GuardQualifiedCurrent(fixed.ManagedRoot, fixed.DataRoot, state.Binding, time.Now(), func() error {
			status, err := installer.ReadClientsSeeded(ctx, clients)
			if err != nil || status.State != "current_materialization" || status.Record == nil {
				return errors.New("current admitted document workspace/carrier unavailable")
			}
			if status.Record.Binding.Workspace != selection.Workspace {
				return errors.New("document workspace differs from current carrier")
			}
			result.WorkspaceID = status.Record.Binding.WorkspaceID
			if err := installer.VerifyExperimentalInstalledBinding(fixed); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			result.State, err = prepareFixedPilotDocument(selection.Workspace)
			return err
		})
		if err != nil {
			return err
		}
		if err := installer.VerifyExperimentalInstalledBinding(fixed); err != nil {
			result.State = "fixture_pending_preserved"
			return err
		}
		return nil
	}()
	if encodeErr := json.NewEncoder(output).Encode(result); encodeErr != nil {
		return encodeErr
	}
	return err
}

func verifyPublicPilotDocument(file *os.File, expected []byte) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != int64(len(expected)) {
		return errors.New("existing pilot document is not the exact regular public fixture; preserved")
	}
	if err := scheduler.ValidateSingleLink(file); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	body, err := io.ReadAll(io.LimitReader(file, int64(len(expected))+1))
	if err != nil {
		return err
	}
	if len(body) != len(expected) || fmt.Sprintf("%x", sha256.Sum256(body)) != pilotdocument.SHA256 {
		return errors.New("existing pilot document differs; preserved without replacement")
	}
	after, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() != after.Size() || info.Mode() != after.Mode() || !info.ModTime().Equal(after.ModTime()) {
		return errors.New("pilot document changed during verification; preserved")
	}
	if err := scheduler.ValidateSingleLink(file); err != nil {
		return err
	}
	return nil
}

func verifyCurrentPilotDocument(file *os.File, open func(int) (*os.File, error), body []byte) error {
	current, err := open(os.O_RDONLY)
	if err != nil {
		return err
	}
	defer current.Close()
	before, err := file.Stat()
	if err != nil {
		return err
	}
	after, err := current.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(before, after) {
		return errors.New("pilot document path changed; all files preserved")
	}
	return verifyPublicPilotDocument(current, body)
}

func prepareFixedPilotDocument(workspace string) (string, error) {
	body, err := pilotdocument.Bytes()
	if err != nil {
		return "fixture_not_prepared", err
	}
	state := "fixture_not_prepared"
	err = withPilotDocumentRoot(workspace, func(open func(int) (*os.File, error)) error {
		file, err := open(os.O_RDONLY)
		if err == nil {
			defer file.Close()
			if err := verifyPublicPilotDocument(file, body); err != nil {
				return err
			}
			if err := verifyCurrentPilotDocument(file, open, body); err != nil {
				return err
			}
			state = "fixture_already_present"
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		file, err = open(os.O_RDWR | os.O_CREATE | os.O_EXCL)
		if err != nil {
			return err
		}
		defer file.Close()
		// Preserve a partial new file on failure. Never remove or replace a leaf
		// by pathname after another application could have changed it.
		state = "fixture_pending_preserved"
		if _, err := file.Write(body); err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			return err
		}
		if err := verifyPublicPilotDocument(file, body); err != nil {
			return err
		}
		if err := verifyCurrentPilotDocument(file, open, body); err != nil {
			return err
		}
		state = "fixture_prepared"
		return nil
	})
	if err != nil && state == "fixture_prepared" {
		state = "fixture_pending_preserved"
	}
	return state, err
}

func verifyPilotDocumentDirectory(path string, held *os.File) error {
	before, err := held.Stat()
	if err != nil {
		return err
	}
	current, err := os.Lstat(filepath.Clean(path))
	if err != nil {
		return err
	}
	if !before.IsDir() || !current.IsDir() || !os.SameFile(before, current) {
		return errors.New("pilot workspace namespace changed; all files preserved")
	}
	return nil
}
