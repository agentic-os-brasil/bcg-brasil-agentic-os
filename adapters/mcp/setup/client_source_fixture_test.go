package setup_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const currentMethodMarker = "CLIENT_CURRENT_SERVER_METHOD_N1_LITERAL"

// Snapshot only exact ordinary Go build inputs (including embed inputs) and the
// closed factory allowlist. No repository/cache/history/private-state copy and
// no source/baseline mutation is used to distinguish future server resources.
func clientFutureSource(t *testing.T, ctx context.Context, source, root string) string {
	t.Helper()
	command := exec.CommandContext(ctx, "go", "list", "-mod=readonly", "-deps", "-json", "./cmd/maestro-mcp", "./cmd/maestro-install")
	command.Dir = filepath.Join(source, "adapters/mcp")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("enumerate finite actual build closure: %s %v", output, err)
	}
	if len(output) > 24<<20 {
		t.Fatal("Go build closure metadata exceeds bound")
	}
	paths := map[string]bool{"go.mod": true, "go.sum": true, "adapters/mcp/go.mod": true, "adapters/mcp/go.sum": true, "bundles/base/distribution.json": true, "adapters/mcp/packaging/testdata/method-ids.txt": true}
	decoder := json.NewDecoder(bytes.NewReader(output))
	for {
		var p struct {
			Dir                                           string
			GoFiles, CgoFiles, SFiles, HFiles, EmbedFiles []string
		}
		err = decoder.Decode(&p)
		if errors.Is(err, io.EOF) {
			break
		}
		must(t, err)
		relative, e := filepath.Rel(source, p.Dir)
		must(t, e)
		if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		for _, files := range [][]string{p.GoFiles, p.CgoFiles, p.SFiles, p.HFiles, p.EmbedFiles} {
			for _, name := range files {
				rel := filepath.ToSlash(filepath.Join(relative, name))
				if strings.HasPrefix(rel, "../") {
					t.Fatal("source closure escaped product")
				}
				paths[rel] = true
			}
		}
	}
	var allowlist struct {
		Schema int `json:"schema_version"`
		Files  []struct {
			Source string `json:"source"`
			Path   string `json:"path"`
		} `json:"files"`
	}
	must(t, json.Unmarshal(read(t, filepath.Join(source, "bundles/base/distribution.json")), &allowlist))
	if allowlist.Schema != 1 {
		t.Fatal("unexpected closed factory source list")
	}
	for _, f := range allowlist.Files {
		paths[f.Source] = true
	}
	if len(paths) > 4096 {
		t.Fatal("fixture source-file budget")
	}
	destination := filepath.Join(root, "future-server-source")
	must(t, os.Mkdir(destination, 0700))
	inputs := map[string]any{}
	var total int64
	for relative := range paths {
		if !filepath.IsLocal(relative) || strings.Contains(relative, ".git/") || strings.Contains(relative, ".superpowers/") || strings.Contains(relative, "node_modules/") {
			t.Fatalf("non-source fixture input %q", relative)
		}
		original := filepath.Join(source, filepath.FromSlash(relative))
		file, e := scheduler.OpenRegularFile(original)
		must(t, e)
		info, e := file.Stat()
		must(t, e)
		system := reflect.ValueOf(info.Sys())
		if system.Kind() == reflect.Pointer {
			system = system.Elem()
		}
		if system.Kind() == reflect.Struct {
			links := system.FieldByName("Nlink")
			if links.IsValid() && links.CanUint() && links.Uint() != 1 {
				t.Fatal("fixture source alias refused")
			}
		}
		body, e := io.ReadAll(io.LimitReader(file, 4<<20+1))
		must(t, e)
		must(t, file.Close())
		if len(body) > 4<<20 {
			t.Fatal("fixture source per-file bound")
		}
		total += int64(len(body))
		if total > 64<<20 {
			t.Fatal("fixture source total bound")
		}
		target := filepath.Join(destination, filepath.FromSlash(relative))
		must(t, os.MkdirAll(filepath.Dir(target), 0700))
		must(t, os.WriteFile(target, body, info.Mode().Perm()))
		inputs[relative] = map[string]any{"bytes": len(body), "sha256": sum(body), "mode": uint32(info.Mode().Perm())}
	}
	doctor := filepath.Join(destination, "bundles/base/skills/maestro-doctor/SKILL.md")
	original := read(t, doctor)
	variant := append(append([]byte{}, original...), []byte("\nControlled engineering source fixture: "+currentMethodMarker+"\n")...)
	must(t, os.WriteFile(doctor, variant, 0600))
	report, _ := json.MarshalIndent(map[string]any{"source": "finite actual Go build/embed closure plus closed factory allowlist", "command": command.Args, "cwd": command.Dir, "input_files": inputs, "total_bytes": total, "original_doctor_sha256": sum(original), "fixture_doctor_sha256": sum(variant), "only_content_variant": "bundles/base/skills/maestro-doctor/SKILL.md", "marker": currentMethodMarker, "source_mutation": false}, "", "  ")
	must(t, os.WriteFile(filepath.Join(root, "future-source-inputs.json"), append(report, '\n'), 0600))
	return destination
}
