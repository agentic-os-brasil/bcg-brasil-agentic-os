package docling

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func settingsCompatibilityFixture(t *testing.T) (targetInputs, map[string]entry) {
	t.Helper()
	inputs := os.Getenv("MAESTRO_WINDOWS_DOCLING_ASSEMBLY_INPUTS")
	if inputs == "" {
		t.Skip("explicit authenticated public wheel required; no interpreter is executed")
	}
	body, err := os.ReadFile(filepath.Join(inputs, "pydantic_settings-2.16.2-py3-none-any.whl"))
	if err != nil || digest(body) != "8d523e5c129b097686bde019bd6109bff53f4c8c3f63f1ddf9933ffa42e5ab4f" {
		t.Fatalf("pinned public wheel unavailable: %v", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]entry{}
	for _, item := range archive.File {
		if item.FileInfo().IsDir() {
			continue
		}
		r, err := item.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(r)
		closeErr := r.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("public wheel member unreadable: %v %v", err, closeErr)
		}
		entries["site-packages/"+item.Name] = entry{body, 0444}
	}
	target, err := inputsForProfile("windows", "amd64", DocumentsProfile)
	if err != nil {
		t.Fatal(err)
	}
	return target, entries
}

func TestWindowsSettingsCompatibilityPreservesPinnedPackage(t *testing.T) {
	target, entries := settingsCompatibilityFixture(t)
	before := make(map[string]entry, len(entries))
	for name, item := range entries {
		before[name] = item
	}
	if err := addWindowsSettingsCompatibility(target, entries); err != nil {
		t.Fatal(err)
	}
	if !windowsSettingsCompatibilityValid(target, entries) || len(entries) != len(before)+1 {
		t.Fatal("transformed inventory contract invalid")
	}
	for name, item := range before {
		if name != settingsMainPath && name != settingsRecordPath && !reflect.DeepEqual(entries[name], item) {
			t.Fatalf("unrelated upstream member changed: %s", name)
		}
	}
	body := entries[settingsMainPath].body
	if bytes.Contains(body, []byte("\nimport asyncio\n")) || !bytes.Contains(body, []byte("        if inspect.iscoroutinefunction(command):\n            import asyncio\n\n")) || digest(body) != "ba6767448d4f690e8a87440910800231f50da10703cd457f4e0613a5adde3336" {
		t.Fatal("genuine asyncio import was not deferred only to coroutine CLI")
	}
	if digest(entries[settingsRecordPath].body) != "da6a8b905d49193fd75fcd05ff8f882ece1f084e0e818d98cf1901c56136d82d" {
		t.Fatal("installed RECORD differs from exact single-row correction")
	}
	for _, name := range []string{settingsMainPath, settingsRecordPath, settingsCompatibilityPath} {
		original := entries[name]
		entries[name] = entry{append(append([]byte{}, original.body...), 'x'), original.mode}
		if windowsSettingsCompatibilityValid(target, entries) {
			t.Fatalf("verifier accepted changed compatibility member: %s", name)
		}
		entries[name] = original
	}
}

func TestWindowsSettingsCompatibilityRefusesChangedInputsBeforeMutation(t *testing.T) {
	for _, change := range []string{"source", "record", "missing", "mode", "collision", "version", "wheel", "duplicate"} {
		t.Run(change, func(t *testing.T) {
			target, entries := settingsCompatibilityFixture(t)
			switch change {
			case "source", "record":
				name := settingsMainPath
				if change == "record" {
					name = settingsRecordPath
				}
				entries[name] = entry{append(append([]byte{}, entries[name].body...), 'x'), 0444}
			case "missing":
				delete(entries, settingsMainPath)
			case "mode":
				entries[settingsMainPath] = entry{entries[settingsMainPath].body, 0555}
			case "collision":
				entries[settingsCompatibilityPath] = entry{[]byte("existing"), 0444}
			default:
				var inputs []Input
				if err := json.Unmarshal(target.inputs, &inputs); err != nil {
					t.Fatal(err)
				}
				for i := range inputs {
					if inputs[i].Name == "pydantic-settings" {
						switch change {
						case "version":
							inputs[i].Version = "2.16.3"
						case "wheel":
							inputs[i].SHA256 = "changed"
						case "duplicate":
							inputs = append(inputs, inputs[i])
						}
						break
					}
				}
				target.inputs, _ = json.Marshal(inputs)
			}
			before := make(map[string]entry, len(entries))
			for name, item := range entries {
				before[name] = item
			}
			if addWindowsSettingsCompatibility(target, entries) == nil || !reflect.DeepEqual(entries, before) {
				t.Fatal("changed upstream accepted or partial compatibility entries published")
			}
		})
	}
}

func TestWindowsSettingsCompatibilityLeavesOtherTargetsUnchanged(t *testing.T) {
	for _, platform := range [][3]string{{"darwin", "arm64", DocumentsProfile}, {"windows", "amd64", LegacyProfile}} {
		target, err := inputsForProfile(platform[0], platform[1], platform[2])
		if err != nil {
			t.Fatal(err)
		}
		entries := map[string]entry{"unchanged": {[]byte("upstream"), 0444}}
		before := map[string]entry{"unchanged": entries["unchanged"]}
		if err := addWindowsSettingsCompatibility(target, entries); err != nil || !reflect.DeepEqual(entries, before) || !windowsSettingsCompatibilityValid(target, entries) {
			t.Fatal("out-of-scope target changed")
		}
	}
}
