package packaging

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeagents"
)

// DesktopFrontendOptions selects only a fresh factory output and public version.
// It cannot select a runtime, namespace, product binding or installation profile.
type DesktopFrontendOptions struct {
	Destination, Version string
	LiveMethods          bool
}

// DesktopFrontendFile describes one public payload resource.
type DesktopFrontendFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

// DesktopFrontendReport describes content integrity, never publisher or host trust.
type DesktopFrontendReport struct {
	BootstrapProtocol   string `json:"bootstrap_protocol,omitempty"`
	SchemaVersion       int    `json:"schema_version"`
	Name                string `json:"name"`
	Version             string `json:"version"`
	Mode                string `json:"mode"`
	NativeQualification string `json:"native_qualification"`
	InventorySHA256     string `json:"inventory_sha256"`
	FileCount           int    `json:"file_count"`
	PayloadBytes        int    `json:"payload_bytes"`
	Archive             string `json:"archive"`
	ArchiveSHA256       string `json:"archive_sha256"`
}

func desktopFrontendPayload(version string) (map[string][]byte, error) {
	if !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(version) || len(version) > 64 {
		return nil, errors.New("numeric semantic version required")
	}
	files, err := methodPayload(claudeagents.DesktopMethodNamespace)
	if err != nil {
		return nil, err
	}
	agents, err := baseagents.Files()
	if err != nil {
		return nil, err
	}
	for name, body := range agents {
		files["managed/agents/"+name] = body
	}
	files["skills/operator/SKILL.md"], files["skills/resume/SKILL.md"] = []byte(operator), []byte(resume)
	manifest, err := json.MarshalIndent(map[string]any{
		"name": claudeagents.DesktopMethodNamespace, "version": version,
		"description": "Canonical Maestro skills and reference resources; connect separately to an admitted Maestro extension. Native execution is not qualified by this package.",
		"author":      map[string]string{"name": "Agentic OS Brasil"},
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	files[".claude-plugin/plugin.json"] = append(manifest, '\n')
	files["README.md"] = []byte(desktopFrontendReadme)
	return files, nil
}

const desktopFrontendReadme = `# Maestro Desktop method instructions

This plugin contains canonical skills, method policy and agent reference documents.
Its separate maestro-desktop-methods identity preserves the maestro-product Code
plugin identity. Account installation may also synchronize these instructions to
Code; actual coexistence and discovery still require host acceptance.

This is an instruction/resource frontend. It contains no MCP configuration,
server, executable, native agent projection or hooks. managed/agents/ contains
reference documents, not native runners. Qualified native Code capabilities in
canonical recipes do not become qualified in Chat or Cowork through packaging.
Methods requiring unavailable tools or native delegation must stop that route.

Use the separately selected local Maestro extension only when this host actually
exposes it. Confirm the admitted workspace ID with maestro_context and read the
complete methods and agent-skill policy references before a method. This plugin
creates no duplicate MCP connector and never selects owner or workspace paths.
A missing connector, scope mismatch or unavailable trusted review stops the route.

Onboarding and protected changes require successful review through the existing
admitted Desktop review channel. Backend command-line conversion or tests do not
prove graphical owner review. Preserve private bindings, participant packets and
credentials in the admitted local installation; no Code binding or blanket
approval may substitute for an unavailable host/review path.

Chat and Cowork have different capabilities. This frontend does not qualify
native delegation, hooks, scheduled execution or full mission parity on either.
Keep LICENSE.md and pocock-upstream.json when redistributing these resources.
Inventory hashes describe bytes, not publisher authority, installation or approval.
`

// BuildDesktopMethodFrontend creates a fresh, content-only factory directory.
// It never opens product/owner/workspace state or invokes the installed backend.
func BuildDesktopMethodFrontend(o DesktopFrontendOptions) (DesktopFrontendReport, error) {
	report := DesktopFrontendReport{SchemaVersion: 1, Name: claudeagents.DesktopMethodNamespace, Version: o.Version, Mode: "instructions_only", NativeQualification: "unavailable"}
	files, err := desktopFrontendPayload(o.Version)
	if err == nil && o.LiveMethods {
		files, err = desktopLiveFrontendPayload(o.Version)
		report.BootstrapProtocol = DesktopLiveMethodProtocol
	}
	if err != nil {
		return report, err
	}
	if !filepath.IsAbs(o.Destination) || filepath.Clean(o.Destination) != o.Destination || filepath.Base(o.Destination) == "." {
		return report, errors.New("clean absolute fresh frontend destination required")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(o.Destination))
	if err != nil {
		return report, err
	}
	if parent != filepath.Dir(o.Destination) {
		return report, errors.New("frontend output parent must use its canonical path")
	}
	if _, err := os.Lstat(o.Destination); err == nil {
		return report, errors.New("frontend destination already exists; preserved")
	} else if !errors.Is(err, os.ErrNotExist) {
		return report, err
	}
	stage, err := os.MkdirTemp(parent, ".maestro-method-frontend-")
	if err != nil {
		return report, err
	}
	created, err := os.Lstat(stage)
	if err != nil {
		return report, err
	}
	defer func() {
		if now, e := os.Lstat(stage); e == nil && os.SameFile(created, now) {
			_ = os.RemoveAll(stage)
		}
	}()
	root, err := os.OpenRoot(stage)
	if err != nil {
		return report, err
	}
	var closed bool
	defer func() {
		if !closed {
			_ = root.Close()
		}
	}()
	write := func(name string, body []byte) error {
		if !fs.ValidPath(name) {
			return errors.New("invalid frontend output path")
		}
		if err := root.MkdirAll(filepath.Dir(name), 0755); err != nil {
			return err
		}
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return err
		}
		_, writeErr := file.Write(body)
		return errors.Join(writeErr, file.Sync(), file.Close())
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	inventory := make([]DesktopFrontendFile, 0, len(names))
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	for _, name := range names {
		body := files[name]
		if err := write(report.Name+"/"+name, body); err != nil {
			_ = z.Close()
			return report, err
		}
		inventory = append(inventory, DesktopFrontendFile{Path: name, SHA256: fmt.Sprintf("%x", sha256.Sum256(body)), Bytes: len(body)})
		report.PayloadBytes += len(body)
		header := &zip.FileHeader{Name: report.Name + "/" + name, Method: zip.Deflate}
		header.SetMode(0644)
		header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
		entry, err := z.CreateHeader(header)
		if err != nil {
			_ = z.Close()
			return report, err
		}
		if _, err = entry.Write(body); err != nil {
			_ = z.Close()
			return report, err
		}
	}
	if err := z.Close(); err != nil {
		return report, err
	}
	inventoryBody, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return report, err
	}
	inventoryBody = append(inventoryBody, '\n')
	report.InventorySHA256 = fmt.Sprintf("%x", sha256.Sum256(inventoryBody))
	report.FileCount = len(names)
	report.Archive = report.Name + "-" + o.Version + ".zip"
	report.ArchiveSHA256 = fmt.Sprintf("%x", sha256.Sum256(archive.Bytes()))
	provenance, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return report, err
	}
	provenance = append(provenance, '\n')
	for _, out := range []struct {
		name string
		body []byte
	}{{"inventory.json", inventoryBody}, {report.Archive, archive.Bytes()}, {"provenance.json", provenance}} {
		if err := write(out.name, out.body); err != nil {
			return report, err
		}
	}
	sums := fmt.Sprintf("%x  inventory.json\n%x  provenance.json\n%x  %s\n", sha256.Sum256(inventoryBody), sha256.Sum256(provenance), sha256.Sum256(archive.Bytes()), report.Archive)
	if err := write("SHA256SUMS", []byte(sums)); err != nil {
		return report, err
	}
	if err := root.Close(); err != nil {
		closed = true
		return report, err
	}
	closed = true
	if err := publishPacket(stage, o.Destination); err != nil {
		return report, err
	}
	return report, nil
}
