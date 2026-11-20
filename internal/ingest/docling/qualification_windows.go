//go:build windows

package docling

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	runtimeidentity "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userlevel"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

//go:embed runtime/windows-amd64/qualification.docx
var qualificationDOCX []byte

type nativeQualification struct {
	Schema                               int `json:"schema"`
	Pack, Manifest, Implementation, Host string
	Source                               string `json:"source_sha256"`
	KernelDenials                        bool   `json:"kernel_denials"`
	RealSimplePipeline                   bool   `json:"real_simple_pipeline"`
}

func qualificationBinding(o Options, pack verifiedPack) (nativeQualification, error) {
	if err := userlevel.EnsureNotElevated(); err != nil {
		return nativeQualification{}, err
	}
	implementation, err := runtimeidentity.Docling()
	if err != nil {
		return nativeQualification{}, err
	}
	owner, err := winprivate.CurrentUser()
	if err != nil {
		return nativeQualification{}, err
	}
	host, err := os.Hostname()
	if err != nil {
		return nativeQualification{}, err
	}
	version := windows.RtlGetVersion()
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return nativeQualification{}, err
	}
	defer key.Close()
	revision, _, err := key.GetIntegerValue("UBR")
	if err != nil {
		return nativeQualification{}, err
	}
	installed := filepath.Join(o.DataRoot, "ingestion", "docling", "installed", pack.sha)
	held, err := winprivate.OpenPinnedTree(installed)
	if err != nil {
		return nativeQualification{}, err
	}
	defer held.Close()
	var identity windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(held.Handle(), &identity); err != nil {
		return nativeQualification{}, err
	}
	hostDigest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d.%d.%d.%d\x00%d:%d:%d", owner.String(), host, version.MajorVersion, version.MinorVersion, version.BuildNumber, revision, identity.VolumeSerialNumber, identity.FileIndexHigh, identity.FileIndexLow)))
	return nativeQualification{Schema: 1, Pack: pack.sha, Manifest: pack.manifestSHA, Implementation: implementation, Host: hex.EncodeToString(hostDigest[:]), Source: digest(qualificationDOCX), KernelDenials: true, RealSimplePipeline: true}, nil
}

// qualificationStatus reads only. It never launches, repairs, prompts or creates
// a Windows profile; an absent/stale proof requires explicit reviewed setup.
func qualificationStatus(ctx context.Context, o Options, pack verifiedPack) string {
	if ctx.Err() != nil {
		return "conversion_cancelled"
	}
	expected, err := qualificationBinding(o, pack)
	if err != nil {
		return "requalification_required"
	}
	name, err := qualificationPath(o, pack, expected)
	if err != nil {
		return "requalification_required"
	}
	ciphertext, err := winprivate.ReadFile(name, 64<<10)
	if err != nil || len(ciphertext) == 0 {
		return "requalification_required"
	}
	input := windows.DataBlob{Size: uint32(len(ciphertext)), Data: &ciphertext[0]}
	var output windows.DataBlob
	if err = windows.CryptUnprotectData(&input, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output); err != nil {
		return "requalification_required"
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))
	if output.Size == 0 || output.Size > 64<<10 {
		return "requalification_required"
	}
	var actual nativeQualification
	if decode(unsafe.Slice(output.Data, int(output.Size)), &actual) != nil || actual != expected {
		return "requalification_required"
	}
	return ""
}

// qualifyPlaced is called only by explicit authenticated placement. The real
// converter and kernel negative controls must succeed before any proof is sealed.
func qualifyPlaced(ctx context.Context, o Options) error {
	pack, reason := New(o).verify(ctx)
	if reason != "" {
		return fmt.Errorf("placed converter unavailable: %s", reason)
	}
	if qualificationStatus(ctx, o, pack) == "" {
		return nil
	}
	binding, err := qualificationBinding(o, pack)
	if err != nil {
		return err
	}
	work, snapshot, err := prepareExecution(o.DataRoot, pack)
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	route, err := ingest.SelectFormat(".docx", false)
	if err != nil {
		return err
	}
	request, _ := json.Marshal(bridgeRequest{Source: qualificationDOCX, MaxInput: 1 << 20, MaxOutput: 1 << 20, MaxExpanded: 1 << 20, Format: ".docx", Route: route, Phase: "convert"})
	deadline, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	stdout := boundedOutput{maximum: 2 << 20, cancel: cancel}
	stderr := boundedOutput{maximum: 64 << 10, cancel: cancel}
	if err = runConfinedBridge(deadline, work, snapshot, request, &stdout, &stderr); err != nil {
		facts := []any{"stage", "launch", "error_type", fmt.Sprintf("%T", err), "deadline_exceeded", errors.Is(err, context.DeadlineExceeded), "cancelled", errors.Is(err, context.Canceled), "stdout_bytes", stdout.Len(), "stderr_bytes", stderr.Len()}
		var code syscall.Errno
		if errors.As(err, &code) {
			facts = append(facts, "syscall_code", uint64(code))
		}
		if status := err.Error(); strings.HasPrefix(status, "confined converter exit ") {
			// Keep only the known status line, never joined paths or output bodies.
			status, _, _ = strings.Cut(status, "\n")
			status, _, _ = strings.Cut(status, "stderr=")
			status, _, _ = strings.Cut(status, "stdout=")
			facts = append(facts, "process_status", status[:min(len(status), 256)])
		}
		exception, line := pythonQualificationFailureFacts(stderr.Bytes(), filepath.Join(snapshot, "bridge.py"), pack.target.bridge)
		facts = append(facts, "python_exception", exception, "python_bridge_line", line)
		slog.Debug("native converter qualification failed", facts...)
		return errors.New("native_converter_qualification_failed")
	}
	var response bridgeResponse
	if decode(stdout.Bytes(), &response) != nil || !response.NetworkDenied || response.Refusal != "" || response.DoclingExport != nil || response.Route != route || !strings.Contains(string(response.Markdown), `MAESTRO\_DOCLING\_OFFLINE\_20261101`) || !bytes.Contains(response.Document, []byte("MAESTRO_DOCLING_OFFLINE_20261101")) {
		slog.Debug("native converter qualification failed", "stage", "response")
		return errors.New("native_converter_qualification_failed")
	}
	var document struct {
		Schema string `json:"schema_name"`
		Tables []struct {
			Data struct {
				Rows  int `json:"num_rows"`
				Cols  int `json:"num_cols"`
				Cells []struct {
					Text string `json:"text"`
				} `json:"table_cells"`
			} `json:"data"`
		} `json:"tables"`
	}
	if json.Unmarshal(response.Document, &document) != nil || document.Schema != "DoclingDocument" || len(document.Tables) != 1 || document.Tables[0].Data.Rows != 2 || document.Tables[0].Data.Cols != 2 {
		slog.Debug("native converter qualification failed", "stage", "shape")
		return errors.New("native_converter_qualification_failed")
	}
	cells := map[string]bool{}
	for _, cell := range document.Tables[0].Data.Cells {
		cells[cell.Text] = true
	}
	if len(cells) != 4 || !cells["Item"] || !cells["Count"] || !cells["alpha"] || !cells["7"] {
		slog.Debug("native converter qualification failed", "stage", "cells")
		return errors.New("native_converter_qualification_failed")
	}
	// Revalidate signed bytes and machine/source binding after the actual process.
	current, reason := New(o).verify(ctx)
	if reason != "" || current.sha != pack.sha || current.manifestSHA != pack.manifestSHA {
		return errors.New("qualification_pack_changed")
	}
	after, err := qualificationBinding(o, current)
	if err != nil || after != binding {
		return errors.New("qualification_binding_changed")
	}
	body, _ := json.Marshal(binding)
	input := windows.DataBlob{Size: uint32(len(body)), Data: &body[0]}
	var output windows.DataBlob
	if err = windows.CryptProtectData(&input, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output); err != nil {
		return err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))
	if output.Size == 0 || output.Size > 64<<10 {
		return errors.New("qualification_proof_invalid")
	}
	path, err := qualificationPath(o, pack, binding)
	if err != nil {
		return err
	}
	if o.metadataGeneration != "" {
		if err := scheduler.EnsurePrivateDirectory(filepath.Dir(path)); err != nil {
			return err
		}
		return scheduler.WriteNewPrivateFile(path, unsafe.Slice(output.Data, int(output.Size)))
	}
	return scheduler.WritePrivateFileAtomic(path, unsafe.Slice(output.Data, int(output.Size)))
}

// These are untrusted diagnostic hints from the fixed synthetic qualification,
// never proof. Only a closed exception name and an in-source exact-path line may
// leave this function; messages, paths and output bodies stay private.
func pythonQualificationFailureFacts(stderr []byte, bridgePath string, source []byte) (string, int) {
	exception, bridgeLine := "unknown", 0
	if len(stderr) > 64<<10 || len(source) == 0 {
		return exception, bridgeLine
	}
	lineCount := bytes.Count(source, []byte("\n"))
	if source[len(source)-1] != '\n' {
		lineCount++
	}
	framePrefix := "  File \"" + bridgePath + "\", line "
	for _, raw := range bytes.Split(stderr, []byte("\n")) {
		row := strings.TrimSuffix(string(raw), "\r")
		if row == "Traceback (most recent call last):" {
			exception, bridgeLine = "unknown", 0
			continue
		}
		if rest, ok := strings.CutPrefix(row, framePrefix); ok {
			bridgeLine = 0
			text, function, complete := strings.Cut(rest, ", in ")
			line, err := strconv.Atoi(text)
			if complete && function != "" && err == nil && line > 0 && line <= lineCount && strconv.Itoa(line) == text {
				bridgeLine = line
			}
		}
		if row == "" || row[0] == ' ' || row[0] == '\t' {
			continue
		}
		name, _, _ := strings.Cut(row, ":")
		switch name {
		case "AssertionError", "RuntimeError", "OSError", "PermissionError", "FileNotFoundError", "ModuleNotFoundError", "ImportError", "ValueError", "TypeError", "KeyError", "AttributeError", "SyntaxError", "ConnectionRefusedError", "TimeoutError", "MemoryError":
			exception = name
		default:
			exception = "unknown"
		}
	}
	return exception, bridgeLine
}

func qualificationPath(o Options, pack verifiedPack, binding nativeQualification) (string, error) {
	base, err := metadataBase(o)
	if err != nil {
		return "", err
	}
	legacy := filepath.Join(o.DataRoot, "ingestion", "docling")
	if base == legacy {
		return filepath.Join(base, "windows-qualification.dpapi"), nil
	}
	body, _ := json.Marshal(binding)
	return filepath.Join(base, "qualifications", digest(body)+".dpapi"), nil
}
