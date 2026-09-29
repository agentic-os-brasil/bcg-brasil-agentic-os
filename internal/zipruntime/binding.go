package zipruntime

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"unicode/utf16"
)

var bindingEvents = []string{"SessionStart", "PreToolUse", "PostToolUse", "UserPromptSubmit", "Stop"}

const bindingLabel = "Maestro managed root binding"

func shellLiteral(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func encodedPowerShell(s string) string {
	units := utf16.Encode([]rune(s))
	b := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(b[i*2:], u)
	}
	return "powershell.exe -NoProfile -EncodedCommand " + base64.StdEncoding.EncodeToString(b)
}
func bindingRecord(event, root string) map[string]any {
	command, windows := "exit 0", "powershell.exe -NoProfile -Command \"exit 0\""
	if root == "" && event == "PreToolUse" {
		command = "echo 'Maestro requires attended binding to the confirmed installation root.' >&2; exit 2"
		windows = "powershell.exe -NoProfile -Command \"[Console]::Error.WriteLine('Maestro requires attended binding to the confirmed installation root.'); exit 2\""
	}
	if root != "" {
		command = "/bin/bash " + shellLiteral(filepath.Join(root, ".codex/hooks/run.sh")) + " " + shellLiteral(event)
		script := "& '" + strings.ReplaceAll(filepath.Join(root, ".codex/hooks/run.ps1"), "'", "''") + "' -Event '" + event + "'; exit $LASTEXITCODE"
		windows = encodedPowerShell(script)
	}
	r := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command, "commandWindows": windows, "timeout": float64(10), "statusMessage": bindingLabel}}}
	if event == "PreToolUse" {
		r["matcher"] = ".*"
		if root == "" {
			r["matcher"] = "^(apply_patch|Edit|Write|MultiEdit|NotebookEdit|Agent|spawn_agent)$"
		}
	}
	return r
}

// noBindingAliases rejects aliases in every existing component, not just the leaf.
func noBindingAliases(p string) error {
	p = filepath.Clean(p)
	for {
		info, err := os.Lstat(p)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return errors.New("binding path is a symlink")
		}
		parent := filepath.Dir(p)
		if p == parent {
			return nil
		}
		p = parent
	}
}
func boundedBindingRead(r *os.Root, p string) ([]byte, error) {
	f, e := r.Open(p)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if e != nil || len(b) > 1<<20 {
		return nil, errors.New("binding metadata exceeds bound")
	}
	return b, nil
}

func validateBindingRuntime(r *os.Root, root string) error {
	var m struct {
		SchemaVersion int "json:\"schema_version\""
		Version       string
		Artifacts     []struct{ OS, Arch, Path, SHA256 string }
	}
	b, e := boundedBindingRead(r, "runtime/manifest.json")
	if e != nil || json.Unmarshal(b, &m) != nil || m.SchemaVersion != 1 {
		return errors.New("invalid runtime manifest")
	}
	version, e := boundedBindingRead(r, "VERSION")
	if e != nil || strings.TrimSpace(string(version)) != m.Version || m.Version == "" {
		return errors.New("runtime version mismatch")
	}
	relative := "runtime/" + runtime.GOOS + "-" + runtime.GOARCH + "/maestro-runtime"
	if runtime.GOOS == "windows" {
		relative += ".exe"
	}
	matches := 0
	for _, a := range m.Artifacts {
		if a.OS == runtime.GOOS && a.Arch == runtime.GOARCH {
			matches++
			if a.Path != relative || len(a.SHA256) != 64 {
				return errors.New("invalid runtime artifact")
			}
			if e := noBindingAliases(filepath.Join(root, filepath.FromSlash(a.Path))); e != nil {
				return e
			}
			f, e := r.Open(a.Path)
			if e != nil {
				return e
			}
			h := sha256.New()
			n, e := io.Copy(h, io.LimitReader(f, 256<<20+1))
			f.Close()
			if e != nil || n > 256<<20 || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
				return errors.New("runtime integrity mismatch")
			}
		}
	}
	if matches != 1 {
		return errors.New("runtime artifact missing or ambiguous")
	}
	return nil
}

// BindCodex is an attended setup operation on an explicitly confirmed root.
// It never discovers a root, changes Codex trust, or overrides host permissions.
func BindCodex(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || strings.ContainsAny(root, "\r\n\x00") {
		return errors.New("confirmed absolute installation root required")
	}
	if e := noBindingAliases(root); e != nil {
		return e
	}
	r, e := os.OpenRoot(root)
	if e != nil {
		return e
	}
	defer r.Close()
	for _, p := range []string{"runtime/manifest.json", "VERSION", ".codex/hooks.json", ".codex/hooks/run.sh", ".codex/hooks/run.ps1"} {
		if e = noBindingAliases(filepath.Join(root, p)); e != nil {
			return e
		}
	}
	if e = validateBindingRuntime(r, root); e != nil {
		return e
	}
	for _, p := range []string{".codex/hooks/run.sh", ".codex/hooks/run.ps1"} {
		info, err := r.Stat(p)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("native hook wrapper missing")
		}
	}
	config, e := r.OpenRoot(".codex")
	if e != nil {
		return e
	}
	defer config.Close()
	before, e := boundedBindingRead(config, "hooks.json")
	absent := os.IsNotExist(e)
	if e != nil && !absent {
		return e
	}
	doc := map[string]any{}
	if !absent && (json.Unmarshal(before, &doc) != nil || doc == nil) {
		return errors.New("invalid existing hooks; preserved")
	}
	hooks, ok := doc["hooks"].(map[string]any)
	if doc["hooks"] != nil && !ok {
		return errors.New("invalid existing hooks map; preserved")
	}
	if hooks == nil {
		hooks = map[string]any{}
		doc["hooks"] = hooks
	}
	for _, event := range bindingEvents {
		records, ok := hooks[event].([]any)
		if hooks[event] != nil && !ok {
			return errors.New("invalid existing hook event; preserved")
		}
		found := false
		for i, value := range records {
			record, ok := value.(map[string]any)
			if !ok {
				return errors.New("invalid hook record")
			}
			handlers, _ := record["hooks"].([]any)
			managed := false
			for _, h := range handlers {
				if handler, ok := h.(map[string]any); ok && handler["statusMessage"] == bindingLabel {
					managed = true
				}
			}
			if managed {
				if found || (!reflect.DeepEqual(record, bindingRecord(event, "")) && !reflect.DeepEqual(record, bindingRecord(event, root))) {
					return errors.New("managed hook customized or bound elsewhere; preserved for attended review")
				}
				records[i] = bindingRecord(event, root)
				found = true
			}
		}
		if !found {
			records = append(records, bindingRecord(event, root))
		}
		hooks[event] = records
	}
	after, e := json.MarshalIndent(doc, "", "  ")
	if e != nil {
		return e
	}
	after = append(after, '\n')
	if bytes.Equal(before, after) {
		return nil
	}
	// Pin the .codex directory handle; a later parent alias cannot redirect writes.
	const temp = "hooks.json.maestro-binding.tmp"
	f, e := config.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return errors.New("binding already pending; existing files preserved")
	}
	defer config.Remove(temp)
	_, e = f.Write(after)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	current, ce := boundedBindingRead(config, "hooks.json")
	if absent {
		if !os.IsNotExist(ce) {
			return errors.New("hooks changed during binding; preserved")
		}
	} else if ce != nil || !bytes.Equal(current, before) {
		return errors.New("hooks changed during binding; preserved")
	}
	return config.Rename(temp, "hooks.json")
}
