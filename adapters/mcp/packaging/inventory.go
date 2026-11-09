package packaging

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const InventoryName = "packet-inventory.json"
const maximumPacketBytes int64 = 512 << 20
const maximumPacketFiles = 4096

type packetFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	Mode   uint32 `json:"mode"`
}
type packetInventory struct {
	SchemaVersion int                  `json:"schema_version"`
	Format        string               `json:"format"`
	Version       string               `json:"version"`
	Platform      string               `json:"platform"`
	Server        string               `json:"server"`
	Files         []packetFile         `json:"files"`
	Stable        *stablePacketBinding `json:"stable,omitempty"`
}

// PacketStatus is integrity evidence only, never publisher or owner authority.
type PacketStatus struct {
	Files                int            `json:"files"`
	Format               string         `json:"format"`
	Version              string         `json:"version"`
	Target               Target         `json:"target"`
	ServerSHA256         string         `json:"server_sha256"`
	RegistrySHA256       string         `json:"registry_sha256,omitempty"`
	InventorySHA256      string         `json:"inventory_sha256"`
	NativeMode           string         `json:"native_mode"`
	Stable               *StableCarrier `json:"stable,omitempty"`
	EmbeddedInputsSHA256 string         `json:"embedded_inputs_sha256,omitempty"`
	NativeContractSHA256 string         `json:"native_contract_sha256,omitempty"`
}

type packetSource struct {
	read       func(string, int64) ([]byte, error)
	scan       func() ([]packetFile, error)
	exactModes bool
}

func directoryPacketSource(root string) packetSource {
	return packetSource{
		read: func(name string, limit int64) ([]byte, error) {
			file, err := scheduler.OpenRegularFile(filepath.Join(root, filepath.FromSlash(name)))
			if err != nil {
				return nil, err
			}
			if err := scheduler.ValidateSingleLink(file); err != nil {
				file.Close()
				return nil, err
			}
			body, readErr := io.ReadAll(io.LimitReader(file, limit+1))
			closeErr := file.Close()
			if readErr != nil || closeErr != nil {
				return nil, errors.Join(readErr, closeErr)
			}
			if int64(len(body)) > limit {
				return nil, errors.New("packet read bound")
			}
			return body, nil
		},
		scan:       func() ([]packetFile, error) { return scanPacket(root) },
		exactModes: runtime.GOOS != "windows",
	}
}

func packetPath(name string) bool {
	if name == "" || filepath.IsAbs(name) || filepath.ToSlash(filepath.Clean(name)) != name || strings.ContainsAny(name, "\\:\x00\r\n") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." || part == "." || part == "" {
			return false
		}
	}
	return true
}
func scanPacket(root string) ([]packetFile, error) {
	var result []packetFile
	var total int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("packet link refused")
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if name == InventoryName {
			return nil
		}
		if !packetPath(name) {
			return errors.New("invalid packet path")
		}
		file, err := scheduler.OpenRegularFile(path)
		if err != nil {
			return err
		}
		if err := scheduler.ValidateSingleLink(file); err != nil {
			file.Close()
			return err
		}
		info, err := file.Stat()
		if err != nil {
			file.Close()
			return err
		}
		if info.Size() < 0 || info.Size() > maximumPacketBytes {
			file.Close()
			return errors.New("packet entry size")
		}
		hash := sha256.New()
		n, readErr := io.Copy(hash, io.LimitReader(file, maximumPacketBytes+1))
		after, statErr := file.Stat()
		closeErr := file.Close()
		if readErr != nil || statErr != nil || closeErr != nil {
			return errors.Join(readErr, statErr, closeErr)
		}
		if n != info.Size() || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) || !os.SameFile(info, after) || after.Mode() != info.Mode() {
			return errors.New("packet changed while reading")
		}
		total += n
		if total > maximumPacketBytes || len(result) >= maximumPacketFiles {
			return errors.New("packet bounds")
		}
		if info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
			return errors.New("packet special mode refused")
		}
		mode := uint32(info.Mode().Perm())
		if runtime.GOOS == "windows" {
			// Windows has no executable permission bit; logical archive modes are
			// independently constrained by the canonical file role below.
			mode = 0644
		}
		result = append(result, packetFile{Path: name, SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: n, Mode: mode})
		return nil
	})
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, err
}
func writeInventory(root, format, version, platform, server string, stable *stablePacketBinding) error {
	files, err := scanPacket(root)
	if err != nil {
		return err
	}
	for i := range files {
		if files[i].Path == server && runtime.GOOS == "windows" {
			files[i].Mode = 0755
		}
	}
	body, err := json.MarshalIndent(packetInventory{SchemaVersion: packetSchema(stable), Format: format, Version: version, Platform: platform, Server: server, Files: files, Stable: stable}, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(root, InventoryName)
	if err := os.WriteFile(path, append(body, '\n'), 0644); err != nil {
		return err
	}
	return os.Chmod(path, 0644)
}

// ValidatePacket reads the complete emitted file set without initialization or
// repair. The inventory excludes itself; it is not a signed trust root.
func ValidatePacket(root string) (PacketStatus, error) {
	return validatePacket(directoryPacketSource(root))
}

func validatePacket(source packetSource) (PacketStatus, error) {
	var status PacketStatus
	body, err := source.read(InventoryName, 1<<20)
	if err != nil {
		return status, err
	}
	if err := packetJSON(body); err != nil {
		return status, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var inventory packetInventory
	if err := decoder.Decode(&inventory); err != nil {
		return status, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return status, errors.New("packet inventory trailing data")
	}
	if inventory.SchemaVersion != packetSchema(inventory.Stable) || (inventory.Format != "plugin" && inventory.Format != "mcpb") || !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(inventory.Version) || len(inventory.Files) < 1 || len(inventory.Files) > maximumPacketFiles || !packetPath(inventory.Server) {
		return status, errors.New("packet inventory unavailable")
	}
	if inventory.Platform != "darwin" && inventory.Platform != "win32" && inventory.Platform != "linux" {
		return status, errors.New("packet platform unavailable")
	}
	if inventory.Stable != nil {
		if err := inventory.Stable.validate(); err != nil {
			return status, err
		}
	}
	expected := map[string]packetFile{}
	for _, file := range inventory.Files {
		if !packetPath(file.Path) || file.Path == InventoryName || file.Bytes < 0 || file.Bytes > maximumPacketBytes || (file.Mode != 0644 && file.Mode != 0755) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(file.SHA256) {
			return status, errors.New("invalid packet entry")
		}
		if _, found := expected[file.Path]; found {
			return status, errors.New("duplicate packet entry")
		}
		expected[file.Path] = file
	}
	if file, found := expected[inventory.Server]; !found || file.Mode != 0755 {
		return status, errors.New("packet server unavailable")
	}
	required, server, err := requiredPacketFiles(inventory.Format, inventory.Platform, inventory.Version, inventory.Stable)
	if err != nil {
		return status, err
	}
	serverBody, err := source.read(server, 128<<20)
	if err != nil {
		return status, err
	}
	_, target, err := inspectNativeBinary(serverBody)
	if err != nil || target.Platform() != inventory.Platform {
		return status, errors.New("packet executable target changed")
	}
	native, err := nativeRequiredPayload(source.read, inventory.Format, target)
	if err != nil {
		return status, err
	}
	for name, body := range native {
		required[name] = body
	}
	if inventory.Server != server {
		return status, errors.New("packet server role changed")
	}
	for name, body := range required {
		file, found := expected[name]
		if !found || file.Mode != 0644 || file.Bytes != int64(len(body)) || file.SHA256 != fmt.Sprintf("%x", sha256.Sum256(body)) {
			return status, fmt.Errorf("canonical packet asset unavailable: %s", name)
		}
	}
	if registry, found := expected["managed/trust/release-authority-registry.json"]; found && (registry.Mode != 0644 || registry.Bytes > 1<<20) {
		return status, errors.New("public registry role changed")
	}
	for name := range expected {
		if name != server && name != "managed/trust/release-authority-registry.json" {
			if _, found := required[name]; !found {
				return status, errors.New("unexpected packet asset")
			}
		}
	}
	actual, err := source.scan()
	if err != nil {
		return status, err
	}
	if len(actual) != len(expected) {
		return status, errors.New("packet file inventory changed")
	}
	for _, file := range actual {
		want, ok := expected[file.Path]
		if !ok || want.SHA256 != file.SHA256 || want.Bytes != file.Bytes || (source.exactModes && want.Mode != file.Mode) {
			return status, fmt.Errorf("packet asset changed: %s", file.Path)
		}
	}
	status = PacketStatus{Files: len(actual), Format: inventory.Format, Version: inventory.Version, Target: target, ServerSHA256: digest(serverBody), InventorySHA256: digest(body)}
	var diagnostic nativePacketManifest
	if err := json.Unmarshal(native[nativeManifestName], &diagnostic); err != nil {
		return PacketStatus{}, err
	}
	status.NativeMode = diagnostic.Mode
	if inventory.Stable != nil {
		if diagnostic.Mode != "mcp_only" || inventory.Stable.BootstrapServerSHA256 != status.ServerSHA256 {
			return PacketStatus{}, errors.New("stable bootstrap identity changed")
		}
		embedded, err := packetEmbeddedDigest(diagnostic.Resources)
		if err != nil || embedded != inventory.Stable.BootstrapEmbeddedInputsSHA256 {
			return PacketStatus{}, errors.New("stable canonical resource identity changed")
		}
		carrier := inventory.Stable.Carrier
		status.Stable = &carrier
		status.EmbeddedInputsSHA256 = embedded
		if inventory.Format == "mcpb" {
			status.NativeContractSHA256 = expected["manifest.json"].SHA256
		}
	}
	if registry, found := expected["managed/trust/release-authority-registry.json"]; found {
		status.RegistrySHA256 = registry.SHA256
	}
	return status, nil
}

// Reject ambiguous inventory objects before the typed closed-schema decode.
func packetJSON(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 8 {
			return errors.New("packet JSON depth")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return errors.New("duplicate packet JSON key")
				}
				seen[name] = true
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return errors.New("invalid packet JSON delimiter")
		}
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("packet JSON trailing data")
	}
	return nil
}
