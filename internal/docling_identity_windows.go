//go:build windows

// Package runtimeidentity identifies the exact shared offline converter sources.
// The ancestor location lets embed consume source bytes without copied snapshots.
package runtimeidentity

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"io/fs"
	"runtime"
	"runtime/debug"
	"sort"
)

//go:embed agentcatalog/catalog.go
//go:embed agentorchestration/controller.go
//go:embed agentorchestration/errors.go
//go:embed agentorchestration/file_privacy_windows.go
//go:embed agentorchestration/filelock.go
//go:embed docling_identity_windows.go
//go:embed ingest/contract.go
//go:embed ingest/docling/assemble.go
//go:embed ingest/docling/confined.go
//go:embed ingest/docling/confined_windows.go
//go:embed ingest/docling/convert.go
//go:embed ingest/docling/files.go
//go:embed ingest/docling/formats.go
//go:embed ingest/docling/installed_windows.go
//go:embed ingest/docling/io_windows.go
//go:embed ingest/docling/native_closure.go
//go:embed ingest/docling/pack.go
//go:embed ingest/docling/place.go
//go:embed ingest/docling/qualification_windows.go
//go:embed ingest/docling/runtime.go
//go:embed ingest/docling/runtime/bridge.py
//go:embed ingest/docling/runtime/inputs.json
//go:embed ingest/docling/runtime/parser-licenses.json
//go:embed ingest/docling/runtime/pylock.toml
//go:embed ingest/docling/runtime/windows-amd64/bridge.py
//go:embed ingest/docling/runtime/windows-amd64/inputs.json
//go:embed ingest/docling/runtime/windows-amd64/pylock.toml
//go:embed ingest/docling/runtime/windows-amd64/native-runtime.json
//go:embed ingest/docling/runtime/windows-amd64/qualification.docx
//go:embed ingest/docling/source.go
//go:embed ingest/docling/target.go
//go:embed ingest/docling/xlsx.go
//go:embed ingest/docling/xlsx_opc.go
//go:embed ingest/formats.go
//go:embed ingest/route.go
//go:embed releasecontract/generation.go
//go:embed releasecontract/manifest.go
//go:embed releasecontract/modern.go
//go:embed releaseverify/generation.go
//go:embed releaseverify/registry.go
//go:embed releaseverify/verify.go
//go:embed scheduler/directory_file_windows.go
//go:embed scheduler/held_directory.go
//go:embed scheduler/held_directory_windows.go
//go:embed scheduler/lease.go
//go:embed scheduler/lease_guard.go
//go:embed scheduler/lease_guard_windows.go
//go:embed scheduler/private_atomic_windows.go
//go:embed scheduler/private_commit.go
//go:embed scheduler/private_path.go
//go:embed scheduler/private_path_api.go
//go:embed scheduler/private_path_windows.go
//go:embed scheduler/private_update.go
//go:embed scheduler/private_update_windows.go
//go:embed scheduler/publish_new_windows.go
//go:embed scheduler/receipt_read.go
//go:embed scheduler/scheduler.go
//go:embed userlevel/userlevel.go
//go:embed userlevel/userlevel_windows.go
//go:embed winprivate/errors.go
//go:embed winprivate/migration_windows.go
//go:embed winprivate/private_windows.go
//go:embed workspace/preflight.go
//go:embed workspace/workspace.go
var sources embed.FS

// Docling hashes actual embedded source bytes and security-relevant dependencies.
// Entry-module identity is deliberately excluded: installer and server share this
// implementation, while release verification authenticates each binary separately.
func Docling() (string, error) {
	hash := sha256.New()
	err := fs.WalkDir(sources, ".", func(name string, item fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if item.IsDir() {
			return nil
		}
		body, err := sources.ReadFile(name)
		if err != nil {
			return err
		}
		hash.Write([]byte(name))
		hash.Write([]byte{0})
		hash.Write(body)
		hash.Write([]byte{0})
		return nil
	})
	if err != nil {
		return "", err
	}
	hash.Write([]byte(runtime.Version()))
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", errors.New("converter_build_identity_unavailable")
	}
	var modules []string
	for _, dependency := range info.Deps {
		if dependency.Path != "golang.org/x/sys" && dependency.Path != "golang.org/x/text" && dependency.Path != "github.com/santhosh-tekuri/jsonschema/v6" {
			continue
		}
		if dependency.Version == "" || dependency.Sum == "" {
			return "", errors.New("converter_security_dependency_identity_unavailable")
		}
		if dependency.Replace != nil {
			return "", errors.New("converter_security_dependency_replaced")
		}
		modules = append(modules, dependency.Path+"@"+dependency.Version+"@"+dependency.Sum)
	}
	if len(modules) != 3 {
		return "", errors.New("converter_security_dependency_identity_unavailable")
	}
	sort.Strings(modules)
	for _, module := range modules {
		hash.Write([]byte{0})
		hash.Write([]byte(module))
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
