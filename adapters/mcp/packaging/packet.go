package packaging

import (
	"bytes"
	"crypto/sha256"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

const EnforcementMinimumVersion = "2.1.295"
const nativeProfile = "maestro_planner"

type Target struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

func (t Target) Platform() string {
	if t.OS == "windows" {
		return "win32"
	}
	return t.OS
}

type NativeCapability struct {
	State            string `json:"state"`
	Reason           string `json:"reason,omitempty"`
	Profile          string `json:"profile"`
	EvidenceClass    string `json:"evidence_class"`
	HostVersion      string `json:"host_version,omitempty"`
	VendorProvenance string `json:"vendor_provenance"`
}

// NativeHost is process-local authority. Only the attended native producer can
// mint it; there is deliberately no receipt decoder or exported constructor.
type NativeHost struct {
	binding, serverSHA, hostPath, hostSHA, profileSHA, version string
	configurationDirectory, configurationSHA                   string
	instructionPaths                                           []string
	evidenceClass                                              string
	ready                                                      bool
}

func nativeCapability(o Options) NativeCapability {
	c := NativeCapability{State: "unavailable", Reason: "native_host_not_qualified", Profile: nativeProfile, EvidenceClass: "not_observed", VendorProvenance: "operator_selected_installation_not_independently_vendor_attested"}
	if h := o.NativeHost; h != nil {
		if h.validate(o) == nil {
			c.State = "qualified"
			c.Reason = ""
			c.EvidenceClass = h.evidenceClass
			c.HostVersion = h.version
		} else {
			c.Reason = "native_admission_stale_or_invalid"
		}
	}
	return c
}
func (h *NativeHost) validate(o Options) error {
	if h == nil || !h.ready || h.evidenceClass != "native_observed" && h.evidenceClass != "engineering_fixture" {
		return errors.New("native admission unavailable")
	}
	binding, err := bindingFingerprint(o.Binding)
	if err != nil || binding != h.binding {
		return errors.New("native binding changed")
	}
	body, target, err := readNativeBinary(o.Server)
	if err != nil || digest(body) != h.serverSHA || target.OS != runtime.GOOS || target.Arch != runtime.GOARCH {
		return errors.New("native server changed")
	}
	host, _, err := nativeHostIdentity(h.hostPath)
	if err != nil || host != h.hostSHA {
		return errors.New("native host changed")
	}
	profile, err := profileFingerprint(o)
	if err != nil || profile != h.profileSHA {
		return errors.New("native profile changed")
	}
	if h.configurationDirectory != "" {
		configuration, err := nativeUserConfiguration(h.configurationDirectory, h.instructionPaths...)
		if err != nil || configuration != h.configurationSHA {
			return errors.New("native user configuration changed")
		}
	} else if h.evidenceClass != "engineering_fixture" {
		return errors.New("native user configuration unavailable")
	}
	return nil
}

// Snapshot the bounded selected instruction closure, never credential files.
func nativeUserConfiguration(directory string, paths ...string) (string, error) {
	return instructionConfiguration(directory, paths...)
}

func digest(body []byte) string { return fmt.Sprintf("%x", sha256.Sum256(body)) }

func readNativeBinary(path string) ([]byte, Target, error) {
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return nil, Target{}, err
	}
	defer f.Close()
	if err := scheduler.ValidateSingleLink(f); err != nil {
		return nil, Target{}, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 128<<20 {
		return nil, Target{}, errors.New("bounded native executable required")
	}
	body, err := io.ReadAll(io.LimitReader(f, (128<<20)+1))
	if err != nil || len(body) > 128<<20 {
		return nil, Target{}, errors.New("native executable size")
	}
	after, err := f.Stat()
	if err != nil || int64(len(body)) != info.Size() || !sameHostMetadata(info, after) {
		return nil, Target{}, errors.New("native executable changed while reading")
	}
	return inspectNativeBinary(body)
}

func inspectNativeBinary(body []byte) ([]byte, Target, error) {
	if len(body) == 0 || len(body) > 128<<20 {
		return nil, Target{}, errors.New("bounded native executable required")
	}
	r := bytes.NewReader(body)
	if m, err := macho.NewFile(r); err == nil && m.Type == macho.TypeExec {
		switch m.Cpu {
		case macho.CpuArm64:
			return body, Target{"darwin", "arm64"}, nil
		case macho.CpuAmd64:
			return body, Target{"darwin", "amd64"}, nil
		}
	}
	if p, err := pe.NewFile(r); err == nil && p.Characteristics&0x0002 != 0 && p.Characteristics&0x2000 == 0 {
		switch p.Machine {
		case pe.IMAGE_FILE_MACHINE_AMD64:
			return body, Target{"windows", "amd64"}, nil
		case pe.IMAGE_FILE_MACHINE_ARM64:
			return body, Target{"windows", "arm64"}, nil
		}
	}
	if e, err := elf.NewFile(r); err == nil && e.Class == elf.ELFCLASS64 && (e.Type == elf.ET_EXEC || e.Type == elf.ET_DYN) {
		switch e.Machine {
		case elf.EM_X86_64:
			return body, Target{"linux", "amd64"}, nil
		case elf.EM_AARCH64:
			return body, Target{"linux", "arm64"}, nil
		}
	}
	return nil, Target{}, errors.New("supported 64-bit native executable required")
}

func bindingFingerprint(o product.Options) (string, error) {
	if _, err := product.Open(o); err != nil {
		return "", err
	}
	h, err := datahome.Resolve(datahome.Options{ProductRoot: o.ProductRoot, UserHome: o.UserHome, LookupEnv: o.LookupEnv})
	if err != nil {
		return "", err
	}
	w, err := workspace.Inspect(o.WorkspacePath, filepath.Join(h.Home, "state"))
	if err != nil {
		return "", err
	}
	body, err := json.Marshal([]any{o.ProductRoot, o.UserHome, o.WorkspacePath, h.Home, w.WorkspaceID, o.AuthorityRegistryPath, o.AuthorityRegistrySHA256})
	if err != nil {
		return "", err
	}
	return digest(body), nil
}

func args(o Options) []string {
	return []string{"serve", "--packet", "--product-root", o.Destination, "--user-home", o.Binding.UserHome, "--workspace", o.Binding.WorkspacePath}
}
func nativeProfileFiles(o Options) (map[string][]byte, error) {
	return nativeProfilePayload(o.target.Platform(), "")
}
func profileFingerprint(o Options) (string, error) {
	files, err := nativeProfileFiles(o)
	if err != nil {
		return "", err
	}
	configuration, err := targetConfigurationSnapshot(o.Binding.WorkspacePath)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(map[string]any{"generated": files, "target_configuration": configuration})
	if err != nil {
		return "", err
	}
	return digest(body), nil
}

// Bind launch-time project instruction alternatives and complete rule membership.
func targetConfigurationSnapshot(workspace string) (map[string]string, error) {
	s := newInstructionSources()
	if err := s.ancestry(workspace, true); err != nil {
		return nil, err
	}
	return s.files, nil
}
