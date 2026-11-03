package docling

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

// Convert consumes a canonical workspace-relative source. The caller's identity
// must still resolve to that exact registered workspace in this runtime's data root.
// Returned document bodies are private data, never receipt/log fields.
func (r *Runtime) Convert(ctx context.Context, source ingest.Request, workspaceID string) (Conversion, error) {
	result := Conversion{Result: ingest.Result{SchemaVersion: ingest.SchemaVersion, Adapter: "docling", Route: "docling_local", Status: ingest.StatusUnavailable, Fidelity: ingest.FidelityUnknown, WorkspaceID: workspaceID}}
	fail := func(status ingest.Status, reason string) (Conversion, error) {
		result.Result.Status = status
		result.Result.Warnings = []string{reason}
		return result, nil
	}
	if !relativeName(source.SourcePath) || !filepath.IsAbs(source.WorkspacePath) || filepath.Clean(source.WorkspacePath) != source.WorkspacePath {
		return fail(ingest.StatusBlocked, "source_scope_invalid")
	}
	if !r.boundWorkspace(source.WorkspacePath, workspaceID) {
		return fail(ingest.StatusBlocked, "workspace_unavailable")
	}
	if source.Policy.MaxInputBytes == 0 && source.Policy.MaxOutputBytes == 0 && source.Policy.AllowedExts == nil && !source.Policy.AllowNetwork && !source.Policy.AllowPlugins {
		source.Policy = ingest.DefaultPolicy()
	}
	canonical := ingest.DefaultPolicy()
	if source.Policy.MaxInputBytes > canonical.MaxInputBytes || source.Policy.MaxOutputBytes > canonical.MaxOutputBytes {
		return fail(ingest.StatusBlocked, "policy_exceeds_canonical_budget")
	}
	original := source.SourcePath
	source.SourcePath = filepath.Join(source.WorkspacePath, filepath.FromSlash(original))
	info, err := source.Validate()
	if err != nil {
		return fail(ingest.StatusBlocked, "source_not_admitted")
	}
	extension := strings.ToLower(filepath.Ext(source.SourcePath))
	if !canonical.AllowedExts[extension] {
		return fail(ingest.StatusUnavailable, "format_unsupported")
	}
	body, opened, err := readFile(source.SourcePath, source.Policy.MaxInputBytes)
	if err != nil || !os.SameFile(info, opened) || !r.boundWorkspace(source.WorkspacePath, workspaceID) {
		return fail(ingest.StatusBlocked, "source_changed_or_unsafe")
	}
	if err := validateFormat(body, extension, source.Policy.MaxOutputBytes); err != nil {
		reason := "source_format_invalid"
		if extension == ".docx" {
			reason = "docx_expansion_invalid"
		}
		return fail(ingest.StatusBlocked, reason)
	}
	var candidate struct {
		SchemaName string `json:"schema_name"`
	}
	if extension == ".json" {
		_ = json.Unmarshal(body, &candidate)
	}
	route, err := ingest.SelectFormat(extension, candidate.SchemaName == "DoclingDocument")
	if err != nil {
		return fail(ingest.StatusUnavailable, "format_unsupported")
	}
	result.Result.Route, result.Result.Parser, result.Result.FallbackReason = route.Route, route.Parser, route.FallbackReason
	result.Result.SourceName = filepath.Base(original)
	result.Result.SourceSHA256 = digest(body)
	result.Result.Format = strings.TrimPrefix(extension, ".")
	pack, reason := r.verify(ctx)
	if reason != "" {
		return fail(ingest.StatusUnavailable, reason)
	}
	executions := filepath.Join(r.options.DataRoot, "ingestion", "docling", "executions")
	if err := scheduler.EnsurePrivateDirectory(executions); err != nil {
		return fail(ingest.StatusUnavailable, "execution_storage_unavailable")
	}
	work, err := os.MkdirTemp(executions, "conversion-")
	if err != nil {
		return fail(ingest.StatusUnavailable, "execution_storage_unavailable")
	}
	defer os.RemoveAll(work)
	snapshot := filepath.Join(work, "pack")
	if err := materialize(pack.entries, snapshot); err != nil {
		return fail(ingest.StatusUnavailable, "execution_snapshot_unavailable")
	}
	for _, dir := range []string{"home", "cache", "models", "tmp", "config", "data"} {
		if err := os.Mkdir(filepath.Join(work, dir), 0700); err != nil {
			return fail(ingest.StatusUnavailable, "execution_storage_unavailable")
		}
	}
	deadline, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	run := func(phase string) (bridgeResponse, error) {
		var response bridgeResponse
		request, _ := json.Marshal(bridgeRequest{body, source.Policy.MaxInputBytes, source.Policy.MaxOutputBytes, source.Policy.MaxOutputBytes, extension, route, phase})
		command := exec.CommandContext(deadline, "/usr/bin/sandbox-exec", "-p", "(version 1)(allow default)(deny network*)", filepath.Join(snapshot, "python/bin/python3.12"), "-I", "-S", "-B", filepath.Join(snapshot, "bridge.py"))
		command.Dir = work
		command.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + filepath.Join(work, "home"), "TMPDIR=" + filepath.Join(work, "tmp"), "XDG_CACHE_HOME=" + filepath.Join(work, "cache"), "XDG_CONFIG_HOME=" + filepath.Join(work, "config"), "XDG_DATA_HOME=" + filepath.Join(work, "data"), "HF_HOME=" + filepath.Join(work, "models"), "HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1", "DOCLING_ARTIFACTS_PATH=" + filepath.Join(work, "models"), "OMP_NUM_THREADS=1", "OPENBLAS_NUM_THREADS=1", "VECLIB_MAXIMUM_THREADS=1", "LANG=en_US.UTF-8"}
		command.Stdin = bytes.NewReader(request)
		stdout := boundedOutput{maximum: 2*source.Policy.MaxOutputBytes + 4096, cancel: cancel}
		stderr := boundedOutput{maximum: 64 << 10, cancel: cancel}
		command.Stdout, command.Stderr = &stdout, &stderr
		command.WaitDelay = time.Second
		if err := command.Run(); err != nil {
			return response, err
		}
		if err := decode(stdout.Bytes(), &response); err != nil {
			return response, errBridgeOutput
		}
		return response, nil
	}
	if extension == ".json" && candidate.SchemaName == "DoclingDocument" {
		// The header is only a candidate. The authenticated schema validator checks the
		// actual document shape; core then selects the approved route from that
		// fact. Ordinary JSON never loses literal handling due to a schema claim.
		classification, err := run("classify_json")
		if err != nil {
			if errors.Is(err, errBridgeOutput) {
				return fail(ingest.StatusDegraded, "converter_classification_invalid")
			}
			return fail(ingest.StatusUnavailable, "offline_conversion_failed")
		}
		if classification.DoclingExport == nil || !classification.NetworkDenied || classification.Refusal != "" || len(classification.Markdown) != 0 || len(classification.Document) != 0 || classification.Route != (ingest.FormatRoute{}) || len(classification.Warnings) != 0 {
			return fail(ingest.StatusDegraded, "converter_classification_invalid")
		}
		route, _ = ingest.SelectFormat(extension, *classification.DoclingExport)
		result.Result.Route, result.Result.Parser, result.Result.FallbackReason = route.Route, route.Parser, route.FallbackReason
	}
	response, err := run("convert")
	if err != nil {
		if errors.Is(err, errBridgeOutput) {
			return fail(ingest.StatusDegraded, "converter_output_invalid")
		}
		return fail(ingest.StatusUnavailable, "offline_conversion_failed")
	}
	if response.Refusal == "output_limit" && len(response.Markdown) == 0 && len(response.Document) == 0 {
		return fail(ingest.StatusDegraded, "output_limit")
	}
	if response.Refusal == "source_format_invalid" && len(response.Markdown) == 0 && len(response.Document) == 0 {
		return fail(ingest.StatusBlocked, "source_format_invalid")
	}
	if response.DoclingExport != nil || response.Refusal != "" || response.Route != route || !response.NetworkDenied || !utf8.Valid(response.Markdown) || !utf8.Valid(response.Document) || !json.Valid(response.Document) || int64(len(response.Markdown)+len(response.Document)) > source.Policy.MaxOutputBytes {
		return fail(ingest.StatusDegraded, "converter_output_invalid")
	}
	var document struct {
		SchemaName string            `json:"schema_name"`
		Texts      []json.RawMessage `json:"texts"`
		Tables     []json.RawMessage `json:"tables"`
	}
	if json.Unmarshal(response.Document, &document) != nil || document.SchemaName != "DoclingDocument" {
		return fail(ingest.StatusDegraded, "converter_document_invalid")
	}
	hasContent := len(document.Texts) > 0 || len(document.Tables) > 0
	if extension == ".xls" || extension == ".xlsx" {
		// Sheet names are context, not cell content. An empty workbook or
		// absent formula caches must not publish an apparently usable artifact.
		hasContent = spreadsheetHasContent(document.Tables)
	}
	if strings.TrimSpace(string(response.Markdown)) == "" || !hasContent {
		return fail(ingest.StatusPartial, "converter_content_empty")
	}
	if !r.boundWorkspace(source.WorkspacePath, workspaceID) {
		return fail(ingest.StatusBlocked, "workspace_changed")
	}
	result.Result.Status = ingest.StatusUsable
	result.Result.Fidelity = route.Fidelity
	if route.FallbackReason != "" {
		result.Result.Warnings = []string{route.FallbackReason}
	}
	for _, warning := range response.Warnings {
		switch warning {
		case "xls_cached_values_only", "xlsx_cached_values_only":
			result.Result.Warnings = append(result.Result.Warnings, warning)
		default:
			return fail(ingest.StatusDegraded, "converter_output_invalid")
		}
	}
	result.Result.OutputBytes = int64(len(response.Markdown) + len(response.Document))
	result.Markdown = response.Markdown
	result.Document = response.Document
	result.Provenance = Provenance{PackSHA256: pack.sha, ManifestSHA256: pack.manifestSHA, MarkdownSHA256: digest(response.Markdown), DocumentSHA256: digest(response.Document), LockSHA256: LockSHA256, Python: PythonVersion, Docling: DoclingVersion, NetworkDenied: true}
	return result, nil
}

func spreadsheetHasContent(tables []json.RawMessage) bool {
	for _, raw := range tables {
		var table struct {
			Data struct {
				Cells []struct {
					Text string `json:"text"`
				} `json:"table_cells"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &table) != nil {
			return false
		}
		for _, cell := range table.Data.Cells {
			if strings.TrimSpace(cell.Text) != "" {
				return true
			}
		}
	}
	return false
}

type bridgeRequest struct {
	Source      []byte             `json:"source"`
	MaxInput    int64              `json:"max_input"`
	MaxOutput   int64              `json:"max_output"`
	MaxExpanded int64              `json:"max_expanded"`
	Format      string             `json:"format"`
	Route       ingest.FormatRoute `json:"selected_route"`
	Phase       string             `json:"phase"`
}

var errBridgeOutput = errors.New("converter_output_invalid")

type bridgeResponse struct {
	Markdown      []byte             `json:"markdown"`
	Document      []byte             `json:"document"`
	NetworkDenied bool               `json:"network_denied"`
	Refusal       string             `json:"refusal,omitempty"`
	Route         ingest.FormatRoute `json:"selected_route"`
	Warnings      []string           `json:"warnings,omitempty"`
	DoclingExport *bool              `json:"docling_export,omitempty"`
}

func (r *Runtime) boundWorkspace(name, id string) bool {
	if id == "" {
		return false
	}
	resolved, err := workspace.ResolveReference(id, r.options.DataRoot)
	if err != nil || resolved != name {
		return false
	}
	state, err := workspace.Inspect(name, r.options.DataRoot)
	return err == nil && state.WorkspaceID == id && state.MetadataStatus == "valid" && state.State == "ready"
}

type boundedOutput struct {
	bytes.Buffer
	maximum int64
	cancel  context.CancelFunc
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	if int64(b.Len())+int64(len(p)) > b.maximum {
		b.cancel()
		return 0, errors.New("process_output_limit")
	}
	return b.Buffer.Write(p)
}
