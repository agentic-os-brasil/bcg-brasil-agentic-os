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
