package docling

import (
	"bytes"
	_ "embed"
	"errors"
)

//go:embed runtime/windows-amd64/native-runtime.json
var windowsNativeRuntime []byte

const msvcpSource = "site-packages/numpy.libs/msvcp140-a4c2229bdc2a2a630acdc095b4d86008.dll"
const msvcpSHA256 = "a4c2229bdc2a2a630acdc095b4d86008e5c3e3bc7773174354f3da4f5beb9cde"

func addNativeClosure(target targetInputs, entries map[string]entry) error {
	if target.os != "windows" {
		return nil
	}
	source, ok := entries[msvcpSource]
	if !ok || len(source.body) != 575056 || digest(source.body) != msvcpSHA256 {
		return errors.New("native_runtime_source_invalid")
	}
	if _, exists := entries["python/msvcp140.dll"]; exists {
		return errors.New("native_runtime_alias_collision")
	}
	// Rtree imports the unmangled basename; keep the authenticated DLL bytes intact.
	entries["python/msvcp140.dll"] = source
	entries["native-runtime.json"] = entry{windowsNativeRuntime, 0444}
	return nil
}

func nativeClosureValid(target targetInputs, entries map[string]entry) bool {
	if target.os != "windows" {
		return true
	}
	source, ok := entries[msvcpSource]
	if !ok || len(source.body) != 575056 || digest(source.body) != msvcpSHA256 {
		return false
	}
	alias, ok := entries["python/msvcp140.dll"]
	return ok && alias.mode == source.mode && bytes.Equal(alias.body, source.body) && bytes.Equal(entries["native-runtime.json"].body, windowsNativeRuntime)
}

// The upstream wheel stays immutable. Only this reviewed Windows Documents
// installation defers an unused asynchronous CLI import; real settings and CLI
// implementations remain intact. The fixed provenance is part of pack.Files.
const settingsMainPath = "site-packages/pydantic_settings/main.py"
const settingsRecordPath = "site-packages/pydantic_settings-2.16.2.dist-info/RECORD"
const settingsCompatibilityPath = "compatibility/pydantic-settings-lazy-asyncio-v1.json"
const settingsWheelSHA = "8d523e5c129b097686bde019bd6109bff53f4c8c3f63f1ddf9933ffa42e5ab4f"
const settingsOriginalSHA = "34eceb2915314cf251a3716e1ad8828f600bd77f74b927a3ba770deb5a918f67"
const settingsPatchedSHA = "ba6767448d4f690e8a87440910800231f50da10703cd457f4e0613a5adde3336"
const settingsOriginalRecordSHA = "51b0f00e10ee778200d28e4c27d257c3a1585284078c11cffebea068689f0395"
const settingsPatchedRecordSHA = "da6a8b905d49193fd75fcd05ff8f882ece1f084e0e818d98cf1901c56136d82d"
const settingsOriginalRecordRow = "pydantic_settings/main.py,sha256=NOzrKRUxTPJRo3FuGtiCj2AL1390uSejuncN61qRj2c,54792\n"
const settingsPatchedRecordRow = "pydantic_settings/main.py,sha256=umdnRI1PaQ6Kh0QJEIACMfUNoQcDzUV_TgYTpa3eMzY,54805\n"
const settingsCompatibility = `{"schema_version":1,"transform":"pydantic-settings-lazy-asyncio-v1","scope":"windows-amd64/model-free-documents-v1","package":"pydantic-settings","version":"2.16.2","upstream_wheel_sha256":"8d523e5c129b097686bde019bd6109bff53f4c8c3f63f1ddf9933ffa42e5ab4f","source":"site-packages/pydantic_settings/main.py","original_sha256":"34eceb2915314cf251a3716e1ad8828f600bd77f74b927a3ba770deb5a918f67","installed_sha256":"ba6767448d4f690e8a87440910800231f50da10703cd457f4e0613a5adde3336","record":"site-packages/pydantic_settings-2.16.2.dist-info/RECORD","original_record_sha256":"51b0f00e10ee778200d28e4c27d257c3a1585284078c11cffebea068689f0395","installed_record_sha256":"da6a8b905d49193fd75fcd05ff8f882ece1f084e0e818d98cf1901c56136d82d","license":"site-packages/pydantic_settings-2.16.2.dist-info/licenses/LICENSE","license_sha256":"eb355a753e020346d33d83bf9769135b89fc610568ac531a01c295bcee7fd998","reason":"Move the genuine asyncio import inside the existing coroutine CLI branch; preserve real BaseSettings, synchronous CLI, package version, metadata and license. No network permission is granted."}
`

func windowsSettingsCompatibility(target targetInputs) bool {
	return target.os == "windows" && target.arch == "amd64" && target.profile == DocumentsProfile
}

func settingsInputValid(target targetInputs) bool {
	var inputs []Input
	if decode(target.inputs, &inputs) != nil {
		return false
	}
	count := 0
	for _, input := range inputs {
		if input.Name == "pydantic-settings" {
			if input.Version != "2.16.2" || input.Filename != "pydantic_settings-2.16.2-py3-none-any.whl" || input.SHA256 != settingsWheelSHA {
				return false
			}
			count++
		}
	}
	return count == 1
}

func addWindowsSettingsCompatibility(target targetInputs, entries map[string]entry) error {
	if !windowsSettingsCompatibility(target) {
		return nil
	}
	source, sourceOK := entries[settingsMainPath]
	record, recordOK := entries[settingsRecordPath]
	_, collision := entries[settingsCompatibilityPath]
	if !settingsInputValid(target) || !sourceOK || !recordOK || collision || source.mode != 0444 || record.mode != 0444 || digest(source.body) != settingsOriginalSHA || digest(record.body) != settingsOriginalRecordSHA {
		return errors.New("settings_compatibility_source_invalid")
	}
	importAnchor := []byte("import asyncio\n")
	branchAnchor := []byte("        if inspect.iscoroutinefunction(command):\n")
	if bytes.Count(source.body, importAnchor) != 1 || bytes.Count(source.body, branchAnchor) != 1 || bytes.Count(record.body, []byte(settingsOriginalRecordRow)) != 1 {
		return errors.New("settings_compatibility_anchor_invalid")
	}
	body := bytes.Replace(source.body, importAnchor, nil, 1)
	body = bytes.Replace(body, branchAnchor, append(append([]byte{}, branchAnchor...), []byte("            import asyncio\n\n")...), 1)
	recordBody := bytes.Replace(record.body, []byte(settingsOriginalRecordRow), []byte(settingsPatchedRecordRow), 1)
	if digest(body) != settingsPatchedSHA || digest(recordBody) != settingsPatchedRecordSHA {
		return errors.New("settings_compatibility_result_invalid")
	}
	// Publish all three entries only after every original/output assertion passes.
	entries[settingsMainPath] = entry{body, source.mode}
	entries[settingsRecordPath] = entry{recordBody, record.mode}
	entries[settingsCompatibilityPath] = entry{[]byte(settingsCompatibility), 0444}
	return nil
}

func windowsSettingsCompatibilityValid(target targetInputs, entries map[string]entry) bool {
	if !windowsSettingsCompatibility(target) {
		return true
	}
	source, sourceOK := entries[settingsMainPath]
	record, recordOK := entries[settingsRecordPath]
	provenance, provenanceOK := entries[settingsCompatibilityPath]
	return settingsInputValid(target) && sourceOK && recordOK && provenanceOK && source.mode == 0444 && record.mode == 0444 && provenance.mode == 0444 && digest(source.body) == settingsPatchedSHA && digest(record.body) == settingsPatchedRecordSHA && bytes.Equal(provenance.body, []byte(settingsCompatibility))
}
