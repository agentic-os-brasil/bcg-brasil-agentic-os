//go:build windows

package docling

// This finite, synthetic assessment belongs only to the original confinement
// parent and its actual children. It grants no export or real-profile authority.
// Custody is cooperative: held handles and readback do not prevent a same-user
// process from rebinding a registry name between observations.

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

const r17DefinitionVersion = "maestro-r17-registry-assessment-v1"
const r17Definition = r17DefinitionVersion + "|HKU:token-user/Software/MaestroR17RegistryAssessmentV1-sha256(clean-work)|volatile,new,64bit,noninheriting|owner=token-user;D:P:OW:20000,user:f003f|owner_only:none:LW;capability_read:registryRead:20009:LW;capability_broad:registryRead:7000f:LW;foreign_capability:internetClient:20009:LW;integrity_pair:registryRead:7000f:ME|label:flags0,NW|sentinel:REG_BINARY,sha256(version,workhash,fixture)|rights:query_value:1,enumerate_sub_keys:8,set_value:2,create_sub_key:4,delete:10000,write_dac:40000|parent:Medium;child:Low;phases:first,second|cooperative-custody;handle-eligibility-only;no-child-mutation"
const r17PassedMarker = `NATIVE_WINDOWS_CONFINEMENT_PASSED {"app_container":true,"children_denied":true,"network_denied":true,"outside_read_denied":true,"outside_write_denied":true,"scratch_writable":true,"snapshot_write_denied":true}`
const r17MarkerName = ".maestro-r17-registry-assessment-v1"

const (
	r17ReadMask  = uint32(0x00020009)
	r17BroadMask = uint32(0x0007000f)
	r17View      = uint32(0x0100)
	// This is a finite protocol failure, not a replacement for an API LSTATUS.
	r17Invalid = uint32(13) // ERROR_INVALID_DATA
)

type r17FixtureDefinition struct {
	name       string
	capability string
	mask       uint32
	label      string
}

var r17Fixtures = [...]r17FixtureDefinition{
	{"owner_only", "", 0, "LW"},
	{"capability_read", "registryRead", r17ReadMask, "LW"},
	{"capability_broad", "registryRead", r17BroadMask, "LW"},
	{"foreign_capability", "internetClient", r17ReadMask, "LW"},
	{"integrity_pair", "registryRead", r17BroadMask, "ME"},
}

var r17Rights = [...]struct {
	name string
	mask uint32
}{
	{"query_value", 0x00000001},
	{"enumerate_sub_keys", 0x00000008},
	{"set_value", 0x00000002},
	{"create_sub_key", 0x00000004},
	{"delete", 0x00010000},
	{"write_dac", 0x00040000},
}

type r17Failure struct {
	stage  string // Only fixed call-site enums enter this field.
	status uint32
}

func (e *r17Failure) Error() string { return fmt.Sprintf("stage=%s status=%d", e.stage, e.status) }

func r17Check(stage string, err error) error {
	if err == nil {
		return nil
	}
	var status syscall.Errno
	if errors.As(err, &status) {
		return &r17Failure{stage, uint32(status)}
	}
	return &r17Failure{stage, r17Invalid}
}

func r17Diagnostic(t *testing.T, err error) {
	t.Helper()
	var failure *r17Failure
	if !errors.As(err, &failure) {
		failure = &r17Failure{"internal", r17Invalid}
	}
	fmt.Fprintf(os.Stdout, "R17_REGISTRY_DIAGNOSTIC stage=%s win32_status=%d\n", failure.stage, failure.status)
	t.Error("finite registry assessment failed; bounded diagnostic retained")
}

// Existing qualification/hosted markers protect ordinary local suites against
// accidental fixture writes. They are cooperative guards, not host attestation.
func r17HostedAssessment(qualification, actions, runner string) bool {
	return qualification == "1" && actions == "true" && runner == "github-hosted"
}

func r17MarkerBody() []byte {
	return []byte(fmt.Sprintf("%s\n%x\n", r17DefinitionVersion, sha256.Sum256([]byte(r17Definition))))
}

func r17CreateMarker(work string) error {
	file, err := os.OpenFile(filepath.Join(work, r17MarkerName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0444)
	if err != nil {
		return r17Check("marker_create", err)
	}
	body := r17MarkerBody()
	n, writeErr := file.Write(body)
	closeErr := file.Close()
	if writeErr != nil {
		return r17Check("marker_write", writeErr)
	}
	if closeErr != nil {
		return r17Check("marker_close", closeErr)
	}
	if n != len(body) {
		return &r17Failure{"marker_short_write", r17Invalid}
	}
	return r17RequireMarker(work)
}

func r17ReadMarker(work string) (bool, error) {
	file, err := os.Open(filepath.Join(work, r17MarkerName))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, r17Check("marker_open", err)
	}
	expected := r17MarkerBody()
	body, readErr := io.ReadAll(io.LimitReader(file, int64(len(expected)+1)))
	closeErr := file.Close()
	if readErr != nil {
		return false, r17Check("marker_read", readErr)
	}
	if closeErr != nil {
		return false, r17Check("marker_read_close", closeErr)
	}
	if !bytes.Equal(body, expected) {
		return false, &r17Failure{"marker_definition", r17Invalid}
	}
	return true, nil
}

func r17RequireMarker(work string) error {
	selected, err := r17ReadMarker(work)
	if err != nil {
		return err
	}
	if !selected {
		return &r17Failure{"marker_missing", r17Invalid}
	}
	return nil
}

// The original launcher sets lpCurrentDirectory to its held work root.
// AppContainer startup can redirect TMP, so it cannot identify that root.
func r17ChildWork() (string, bool, error) {
	work, err := os.Getwd()
	if err != nil {
		return "", false, r17Check("child_work_read", err)
	}
	if !filepath.IsAbs(work) || filepath.Clean(work) != work {
		return "", false, &r17Failure{"child_work_shape", r17Invalid}
	}
	selected, err := r17ReadMarker(work)
	return work, selected, err
}

func r17ChildAssessmentSelected(t *testing.T) bool {
	t.Helper()
	_, selected, err := r17ChildWork()
	if err != nil {
		r17Diagnostic(t, err)
		t.FailNow()
	}
	return selected
}

func r17UserAndIntegrity(wantLabel string) (*windows.SID, error) {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		return nil, r17Check("token_user", err)
	}
	// A fixed buffer is ample for TOKEN_MANDATORY_LABEL and a maximum SID.
	var buffer [256]byte
	var returned uint32
	if err = windows.GetTokenInformation(token, windows.TokenIntegrityLevel, &buffer[0], uint32(len(buffer)), &returned); err != nil {
		return nil, r17Check("token_integrity", err)
	}
	if returned < uint32(unsafe.Sizeof(windows.Tokenmandatorylabel{})) || returned > uint32(len(buffer)) {
		return nil, &r17Failure{"token_integrity_shape", r17Invalid}
	}
	label := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&buffer[0]))
	// Compare against a fixed complete SID, not only its final RID.
	want, err := windows.StringToSid("S-1-16-" + map[string]string{"LW": "4096", "ME": "8192"}[wantLabel])
	if err != nil {
		return nil, r17Check("token_label_definition", err)
	}
	start, end := uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&buffer[0]))+uintptr(returned)
	p := uintptr(unsafe.Pointer(label.Label.Sid))
	if p < start || p+8 > end {
		return nil, &r17Failure{"token_integrity_mismatch", r17Invalid}
	}
	count := *(*byte)(unsafe.Add(unsafe.Pointer(label.Label.Sid), 1))
	if p+8+4*uintptr(count) > end || !label.Label.Sid.IsValid() || !windows.EqualSid(label.Label.Sid, want) {
		return nil, &r17Failure{"token_integrity_mismatch", r17Invalid}
	}
	return user.User.Sid.Copy()
}

func r17WorkHash(work string) (string, error) {
	if !filepath.IsAbs(work) || filepath.Clean(work) != work {
		return "", &r17Failure{"work_shape", r17Invalid}
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(work))), nil
}

func r17RootName(hash string) string { return "MaestroR17RegistryAssessmentV1-" + hash }

func r17Sentinel(hash, fixture string) [32]byte {
	return sha256.Sum256([]byte(r17DefinitionVersion + "\n" + hash + "\n" + fixture))
}

type r17Metadata struct {
	subkeys, maxSubkey, values, maxValueName, maxValue, security uint32
	written                                                      windows.Filetime
}

func r17MetadataFor(key windows.Handle) (r17Metadata, error) {
	var m r17Metadata
	err := windows.RegQueryInfoKey(key, nil, nil, nil, &m.subkeys, &m.maxSubkey, nil, &m.values, &m.maxValueName, &m.maxValue, &m.security, &m.written)
	return m, r17Check("key_metadata", err)
}

// Diagnostics select only fixed field enums; full struct equality remains the
// acceptance guard. No field value, delta, key identity or sentinel is emitted.
func r17LeafMetadataMismatchStage(observed, expected r17Metadata) string {
	stage := ""
	for _, field := range [...]struct {
		different bool
		stage     string
	}{
		{observed.subkeys != expected.subkeys, "leaf_metadata_subkeys_changed"},
		{observed.maxSubkey != expected.maxSubkey, "leaf_metadata_max_subkey_changed"},
		{observed.values != expected.values, "leaf_metadata_values_changed"},
		{observed.maxValueName != expected.maxValueName, "leaf_metadata_max_value_name_changed"},
		{observed.maxValue != expected.maxValue, "leaf_metadata_max_value_bytes_changed"},
		{observed.security != expected.security, "leaf_metadata_security_bytes_changed"},
		{observed.written != expected.written, "leaf_metadata_last_write_changed"},
	} {
		if field.different {
			if stage != "" {
				return "leaf_metadata_multiple_changed"
			}
			stage = field.stage
		}
	}
	return stage
}

type r17FixtureSet struct {
	anchor, root              windows.Handle
	leaves                    [5]windows.Handle
	metadata                  [5]r17Metadata
	rootMetadata              r17Metadata
	user, capability, foreign *windows.SID
	hash, rootName            string
	ready, closed             bool
	childrenStopped           bool
}

// These three APIs are deliberately private to test setup/cleanup. Children
// call only RegOpenKeyEx/RegQueryValueEx/RegCloseKey, never these mutation APIs.
var (
	r17Advapi    = windows.NewLazySystemDLL("advapi32.dll")
	r17CreateKey = r17Advapi.NewProc("RegCreateKeyExW")
	r17SetValue  = r17Advapi.NewProc("RegSetValueExW")
	r17DeleteKey = r17Advapi.NewProc("RegDeleteKeyExW")
)

func r17CreateFresh(parent windows.Handle, name string, sd *windows.SECURITY_DESCRIPTOR) (windows.Handle, error) {
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, r17Check("create_name", err)
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	var key windows.Handle
	var disposition uint32
	status, _, _ := r17CreateKey.Call(uintptr(parent), uintptr(unsafe.Pointer(ptr)), 0, 0, 1, uintptr(windows.KEY_ALL_ACCESS|r17View), uintptr(unsafe.Pointer(&sa)), uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&disposition)))
	runtime.KeepAlive(sd)
	if uint32(status) != 0 {
		return 0, &r17Failure{"create_volatile", uint32(status)}
	}
	if disposition != 1 { // REG_CREATED_NEW_KEY; existing state is never adopted.
		closeErr := windows.RegCloseKey(key)
		if closeErr != nil {
			return 0, r17Check("existing_close", closeErr)
		}
		return 0, &r17Failure{"existing_preserved", uint32(windows.ERROR_ALREADY_EXISTS)}
	}
	return key, nil
}

func (s *r17FixtureSet) descriptor(def *r17FixtureDefinition) (*windows.SECURITY_DESCRIPTOR, error) {
	sddl := "O:" + s.user.String() + "D:P(A;;0x00020000;;;OW)(A;;0x000F003F;;;" + s.user.String() + ")"
	if def != nil && def.capability != "" {
		capability := s.capability
		if def.capability == "internetClient" {
			capability = s.foreign
		}
		sddl += fmt.Sprintf("(A;;0x%08X;;;%s)", def.mask, capability.String())
	}
	sd, err := windows.SecurityDescriptorFromString(sddl)
	return sd, r17Check("descriptor_definition", err)
}

func r17Prepare(work string) (*r17FixtureSet, error) {
	s := &r17FixtureSet{childrenStopped: true}
	var err error
	if s.user, err = r17UserAndIntegrity("ME"); err != nil {
		return s, err
	}
	if s.hash, err = r17WorkHash(work); err != nil {
		return s, err
	}
	s.rootName = r17RootName(s.hash)
	if s.capability, err = registryReadCapabilitySID(); err != nil {
		return s, r17Check("capability_definition", err)
	}
	if s.foreign, err = windows.CreateWellKnownSid(windows.WinCapabilityInternetClientSid); err != nil {
		return s, r17Check("foreign_definition", err)
	}
	if windows.EqualSid(s.capability, s.foreign) {
		return s, &r17Failure{"foreign_not_distinct", r17Invalid}
	}
	anchor, err := windows.UTF16PtrFromString(s.user.String() + `\Software`)
	if err != nil {
		return s, r17Check("anchor_name", err)
	}
	// Open only: missing user/Software ancestors are never created or repaired.
	if err = windows.RegOpenKeyEx(windows.HKEY_USERS, anchor, 0, windows.KEY_CREATE_SUB_KEY|windows.KEY_ENUMERATE_SUB_KEYS|windows.READ_CONTROL|r17View, &s.anchor); err != nil {
		return s, r17Check("anchor_open", err)
	}
	sd, err := s.descriptor(nil)
	if err != nil {
		return s, err
	}
	if s.root, err = r17CreateFresh(s.anchor, s.rootName, sd); err != nil {
		return s, err
	}
	for i := range r17Fixtures {
		def := &r17Fixtures[i]
		sd, err = s.descriptor(def)
		if err != nil {
			return s, err
		}
		if s.leaves[i], err = r17CreateFresh(s.root, def.name, sd); err != nil {
			return s, err
		}
		label, labelErr := windows.SecurityDescriptorFromString("S:(ML;;NW;;;" + def.label + ")")
		if labelErr != nil {
			return s, r17Check("label_definition", labelErr)
		}
		sacl, _, labelErr := label.SACL()
		if labelErr != nil {
			return s, r17Check("label_definition_shape", labelErr)
		}
		err = windows.SetSecurityInfo(s.leaves[i], windows.SE_REGISTRY_KEY, windows.LABEL_SECURITY_INFORMATION, nil, nil, nil, sacl)
		runtime.KeepAlive(label)
		if err != nil {
			return s, r17Check("label_set", err)
		}
		value := r17Sentinel(s.hash, def.name)
		name, _ := windows.UTF16PtrFromString("sentinel")
		status, _, _ := r17SetValue.Call(uintptr(s.leaves[i]), uintptr(unsafe.Pointer(name)), 0, uintptr(windows.REG_BINARY), uintptr(unsafe.Pointer(&value[0])), uintptr(len(value)))
		if uint32(status) != 0 {
			return s, &r17Failure{"sentinel_set", uint32(status)}
		}
		if s.metadata[i], err = r17MetadataFor(s.leaves[i]); err != nil {
			return s, err
		}
	}
	if s.rootMetadata, err = r17MetadataFor(s.root); err != nil {
		return s, err
	}
	if err = s.validate(); err != nil {
		return s, err
	}
	s.ready = true
	return s, nil
}

// Compare every actual ACE in order, in memory; no descriptor/SID is retained.
func (s *r17FixtureSet) validateDescriptor(key windows.Handle, def *r17FixtureDefinition) error {
	info := windows.SECURITY_INFORMATION(windows.OWNER_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION)
	if def != nil {
		info |= windows.LABEL_SECURITY_INFORMATION
	}
	sd, err := windows.GetSecurityInfo(key, windows.SE_REGISTRY_KEY, info)
	if err != nil {
		return r17Check("descriptor_read", err)
	}
	owner, defaulted, err := sd.Owner()
	if err != nil || defaulted || owner == nil || !windows.EqualSid(owner, s.user) {
		return &r17Failure{"owner_mismatch", r17Invalid}
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return &r17Failure{"dacl_unprotected", r17Invalid}
	}
	dacl, defaulted, err := sd.DACL()
	count := uint16(2)
	if def != nil && def.capability != "" {
		count++
	}
	if err != nil || defaulted || dacl == nil || dacl.AceCount != count {
		return &r17Failure{"dacl_shape", r17Invalid}
	}
	ownerRights, err := windows.CreateWellKnownSid(windows.WinCreatorOwnerRightsSid)
	if err != nil {
		return r17Check("owner_rights_definition", err)
	}
	for i := uint32(0); i < uint32(count); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err = windows.GetAce(dacl, i, &ace); err != nil {
			return r17Check("dacl_ace_read", err)
		}
		wantSID, wantMask := ownerRights, uint32(windows.READ_CONTROL)
		if i == 1 {
			wantSID, wantMask = s.user, windows.KEY_ALL_ACCESS
		} else if i == 2 {
			wantSID, wantMask = s.capability, def.mask
			if def.capability == "internetClient" {
				wantSID = s.foreign
			}
		}
		if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != 0 || uint32(ace.Mask) != wantMask || ace.Header.AceSize != uint16(8+wantSID.Len()) || !windows.EqualSid((*windows.SID)(unsafe.Pointer(&ace.SidStart)), wantSID) {
			return &r17Failure{"dacl_ace_mismatch", r17Invalid}
		}
	}
	if def != nil {
		sacl, defaulted, err := sd.SACL()
		if err != nil || defaulted || sacl == nil || sacl.AceCount != 1 {
			return &r17Failure{"label_shape", r17Invalid}
		}
		var ace *windows.ACCESS_ALLOWED_ACE // Mandatory-label ACE has the same fixed layout.
		if err = windows.GetAce(sacl, 0, &ace); err != nil {
			return r17Check("label_read", err)
		}
		want, err := windows.StringToSid("S-1-16-" + map[string]string{"LW": "4096", "ME": "8192"}[def.label])
		if err != nil {
			return r17Check("label_sid_definition", err)
		}
		if ace == nil || ace.Header.AceType != 0x11 || ace.Header.AceFlags != 0 || uint32(ace.Mask) != 1 || ace.Header.AceSize != uint16(8+want.Len()) || !windows.EqualSid((*windows.SID)(unsafe.Pointer(&ace.SidStart)), want) {
			return &r17Failure{"label_mismatch", r17Invalid}
		}
	}
	return nil
}

func r17MatchSentinel(key windows.Handle, expected [32]byte) error {
	name, _ := windows.UTF16PtrFromString("sentinel")
	var body [32]byte
	var kind uint32
	length := uint32(len(body))
	if err := windows.RegQueryValueEx(key, name, nil, &kind, &body[0], &length); err != nil {
		return r17Check("sentinel_read", err)
	}
	if kind != windows.REG_BINARY || length != uint32(len(body)) || body != expected {
		return &r17Failure{"sentinel_mismatch", r17Invalid}
	}
	return nil
}

func (s *r17FixtureSet) validate() error {
	if err := s.validateDescriptor(s.root, nil); err != nil {
		return err
	}
	m, err := r17MetadataFor(s.root)
	if err != nil {
		return err
	}
	if m != s.rootMetadata || m.subkeys != 5 || m.values != 0 {
		return &r17Failure{"root_shape_changed", r17Invalid}
	}
	rootName, _ := windows.UTF16PtrFromString(s.rootName)
	var namedRoot windows.Handle
	if err = windows.RegOpenKeyEx(s.anchor, rootName, 0, windows.READ_CONTROL|windows.KEY_QUERY_VALUE|windows.KEY_ENUMERATE_SUB_KEYS|r17View, &namedRoot); err != nil {
		return r17Check("root_name_readback", err)
	}
	namedErr := s.validateDescriptor(namedRoot, nil)
	namedMetadata, metadataErr := r17MetadataFor(namedRoot)
	closeErr := windows.RegCloseKey(namedRoot)
	if namedErr != nil {
		return namedErr
	}
	if metadataErr != nil {
		return metadataErr
	}
	if namedMetadata != s.rootMetadata {
		return &r17Failure{"root_name_changed", r17Invalid}
	}
	if closeErr != nil {
		return r17Check("root_readback_close", closeErr)
	}
	for i, def := range r17Fixtures {
		if err = s.validateDescriptor(s.leaves[i], &def); err != nil {
			return err
		}
		m, err = r17MetadataFor(s.leaves[i])
		if err != nil {
			return err
		}
		if m != s.metadata[i] || m.subkeys != 0 || m.values != 1 || m.maxValueName != 8 || m.maxValue != 32 {
			return &r17Failure{"leaf_shape_changed", r17Invalid}
		}
		if err = r17MatchSentinel(s.leaves[i], r17Sentinel(s.hash, def.name)); err != nil {
			return err
		}
		name, _ := windows.UTF16PtrFromString(def.name)
		for _, right := range r17Rights {
			access := right.mask | r17View
			if right.name == "query_value" {
				// This parent handle also revalidates owner/DACL/label through
				// the original descriptor reader. Child probes stay exact-right.
				access |= windows.READ_CONTROL
			}
			var opened windows.Handle
			if err = windows.RegOpenKeyEx(s.root, name, 0, access, &opened); err != nil {
				return r17Check("parent_right_control", err)
			}
			var readErr error
			if right.name == "query_value" {
				readErr = s.validateDescriptor(opened, &def)
				if readErr == nil {
					readErr = r17MatchSentinel(opened, r17Sentinel(s.hash, def.name))
				}
				if readErr == nil {
					observed, metadataErr := r17MetadataFor(opened)
					if metadataErr != nil {
						readErr = metadataErr
					} else if observed != s.metadata[i] {
						readErr = &r17Failure{r17LeafMetadataMismatchStage(observed, s.metadata[i]), r17Invalid}
					}
				}
			}
			closeErr := windows.RegCloseKey(opened)
			if readErr != nil {
				return readErr
			}
			if closeErr != nil {
				return r17Check("parent_control_close", closeErr)
			}
		}
	}
	return nil
}

// Cleanup runs after the synchronous original launcher has stopped/reaped each
// child. Ambiguous setup/readback preserves all names; only handles are closed.
func (s *r17FixtureSet) cleanup() (result error) {
	if s.closed {
		return &r17Failure{"cleanup_repeated", r17Invalid}
	}
	s.closed = true
	defer func() {
		for i, key := range s.leaves {
			if key != 0 {
				if err := windows.RegCloseKey(key); err != nil && result == nil {
					result = r17Check("cleanup_leaf_close", err)
				}
				s.leaves[i] = 0
			}
		}
		for _, key := range []windows.Handle{s.root, s.anchor} {
			if key != 0 {
				if err := windows.RegCloseKey(key); err != nil && result == nil {
					result = r17Check("cleanup_anchor_close", err)
				}
			}
		}
		s.root, s.anchor = 0, 0
	}()
	if !s.ready {
		return &r17Failure{"cleanup_incomplete_preserved", r17Invalid}
	}
	if !s.childrenStopped {
		// Early launch/attestation failures can return after requesting a kill
		// without a completed wait. Preserve names instead of assuming reaping.
		return &r17Failure{"cleanup_child_stop_unconfirmed", r17Invalid}
	}
	if err := s.validate(); err != nil {
		return &r17Failure{"cleanup_ambiguous_preserved", r17Invalid}
	}
	for i, def := range r17Fixtures {
		if err := windows.RegCloseKey(s.leaves[i]); err != nil {
			return r17Check("cleanup_leaf_close", err)
		}
		s.leaves[i] = 0
		name, _ := windows.UTF16PtrFromString(def.name)
		status, _, _ := r17DeleteKey.Call(uintptr(s.root), uintptr(unsafe.Pointer(name)), uintptr(r17View), 0)
		if uint32(status) != 0 {
			return &r17Failure{"cleanup_leaf_delete", uint32(status)}
		}
	}
	if err := windows.RegCloseKey(s.root); err != nil {
		return r17Check("cleanup_root_close", err)
	}
	s.root = 0
	name, _ := windows.UTF16PtrFromString(s.rootName)
	status, _, _ := r17DeleteKey.Call(uintptr(s.anchor), uintptr(unsafe.Pointer(name)), uintptr(r17View), 0)
	if uint32(status) != 0 {
		return &r17Failure{"cleanup_root_delete", uint32(status)}
	}
	var remaining windows.Handle
	err := windows.RegOpenKeyEx(s.anchor, name, 0, windows.KEY_QUERY_VALUE|r17View, &remaining)
	if err == nil {
		closeErr := windows.RegCloseKey(remaining)
		if closeErr != nil {
			return r17Check("cleanup_residue_close", closeErr)
		}
		return &r17Failure{"cleanup_residue_preserved", r17Invalid}
	}
	if !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return r17Check("cleanup_absence_unconfirmed", err)
	}
	return nil
}

func (s *r17FixtureSet) finish(t *testing.T) {
	t.Helper()
	if !s.closed {
		if err := s.cleanup(); err != nil {
			r17Diagnostic(t, err)
			return
		}
		fmt.Fprintln(os.Stdout, "R17_REGISTRY_CLEANUP state=complete")
	}
}

type r17Row struct {
	fixture, right string
	status         uint32
	match          bool
}

func r17ObserveChild(t *testing.T) {
	t.Helper()
	user, err := r17UserAndIntegrity("LW")
	if err != nil {
		r17Diagnostic(t, err)
		t.FailNow()
	}
	work, selected, err := r17ChildWork()
	if err == nil && !selected {
		err = &r17Failure{"marker_missing", r17Invalid}
	}
	if err != nil {
		r17Diagnostic(t, err)
		t.FailNow()
	}
	hash, err := r17WorkHash(work)
	if err != nil {
		r17Diagnostic(t, err)
		t.FailNow()
	}
	for _, def := range r17Fixtures {
		name, _ := windows.UTF16PtrFromString(user.String() + `\Software\` + r17RootName(hash) + `\` + def.name)
		for _, right := range r17Rights {
			var opened windows.Handle
			// x/sys preserves RegOpenKeyEx's returned LSTATUS, not GetLastError.
			openErr := windows.RegOpenKeyEx(windows.HKEY_USERS, name, 0, right.mask|r17View, &opened)
			row := r17Row{fixture: def.name, right: right.name}
			if openErr != nil {
				var status syscall.Errno
				if !errors.As(openErr, &status) {
					r17Diagnostic(t, &r17Failure{"child_open_status", r17Invalid})
					t.FailNow()
				}
				row.status = uint32(status)
			} else {
				var readErr error
				if right.name == "query_value" {
					readErr = r17MatchSentinel(opened, r17Sentinel(hash, def.name))
					row.match = readErr == nil
				}
				closeErr := windows.RegCloseKey(opened)
				if readErr != nil || closeErr != nil {
					if readErr == nil {
						readErr = r17Check("child_handle_close", closeErr)
					}
					r17Diagnostic(t, readErr)
					t.FailNow()
				}
			}
			fmt.Fprintf(os.Stdout, "R17_REGISTRY_ROW fixture_enum=%s right_enum=%s win32_status=%d sentinel_match=%t\n", row.fixture, row.right, row.status, row.match)
		}
	}
}

func r17ParseChild(body string) ([]r17Row, error) {
	if len(body) > 65536 || strings.Contains(body, "R17_REGISTRY_DIAGNOSTIC") {
		return nil, &r17Failure{"child_output_invalid", r17Invalid}
	}
	want := make(map[string]bool, 30)
	for _, fixture := range r17Fixtures {
		for _, right := range r17Rights {
			want[fixture.name+"/"+right.name] = true
		}
	}
	var rows []r17Row
	markers := 0
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "NATIVE_WINDOWS_CONFINEMENT_PASSED") {
			if line != r17PassedMarker {
				return nil, &r17Failure{"child_original_marker", r17Invalid}
			}
			markers++
		}
		if !strings.Contains(line, "R17_REGISTRY_") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 5 || parts[0] != "R17_REGISTRY_ROW" || !strings.HasPrefix(parts[1], "fixture_enum=") || !strings.HasPrefix(parts[2], "right_enum=") || !strings.HasPrefix(parts[3], "win32_status=") || !strings.HasPrefix(parts[4], "sentinel_match=") {
			return nil, &r17Failure{"child_row_shape", r17Invalid}
		}
		row := r17Row{fixture: strings.TrimPrefix(parts[1], "fixture_enum="), right: strings.TrimPrefix(parts[2], "right_enum=")}
		statusText := strings.TrimPrefix(parts[3], "win32_status=")
		status, err := strconv.ParseUint(statusText, 10, 32)
		matchText := strings.TrimPrefix(parts[4], "sentinel_match=")
		if err != nil || strconv.FormatUint(status, 10) != statusText || (matchText != "true" && matchText != "false") || !want[row.fixture+"/"+row.right] {
			return nil, &r17Failure{"child_row_value", r17Invalid}
		}
		row.status, row.match = uint32(status), matchText == "true"
		if row.match != (row.right == "query_value" && row.status == 0) {
			return nil, &r17Failure{"child_sentinel_contract", r17Invalid}
		}
		canonical := fmt.Sprintf("R17_REGISTRY_ROW fixture_enum=%s right_enum=%s win32_status=%d sentinel_match=%t", row.fixture, row.right, row.status, row.match)
		if !bytes.Equal([]byte(line), []byte(canonical)) {
			return nil, &r17Failure{"child_row_encoding", r17Invalid}
		}
		delete(want, row.fixture+"/"+row.right)
		rows = append(rows, row)
	}
	if scanner.Err() != nil || len(want) != 0 || len(rows) != 30 || markers != 1 {
		return nil, &r17Failure{"child_matrix_incomplete", r17Invalid}
	}
	return rows, nil
}

// Parser controls execute inside the existing parent and touch no registry.
// They never stand in for the actual two confined-child observations.
func r17AssertProtocol(t *testing.T) {
	t.Helper()
	for _, name := range []string{"absent", "redirected_tmp", "foreign_tmp_marker", "malformed"} {
		t.Run("child_work_"+name, func(t *testing.T) {
			work := t.TempDir()
			t.Chdir(work)
			redirected := filepath.Join(work, "data", "Packages", "synthetic", "AC", "Temp")
			t.Setenv("TMP", redirected)
			markerRoot := work
			if name == "foreign_tmp_marker" {
				markerRoot = filepath.Dir(redirected)
				if err := os.MkdirAll(markerRoot, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if name != "absent" {
				body := r17MarkerBody()
				if name == "malformed" {
					body = []byte("foreign marker")
				}
				if err := os.WriteFile(filepath.Join(markerRoot, r17MarkerName), body, 0600); err != nil {
					t.Fatal(err)
				}
			}
			observed, selected, err := r17ChildWork()
			if observed != work || selected != (name == "redirected_tmp") || (err != nil) != (name == "malformed") {
				t.Fatal("child work locator lost exact launcher-root marker selection")
			}
		})
	}
	metadata := r17Metadata{subkeys: 1, maxSubkey: 2, values: 3, maxValueName: 4, maxValue: 5, security: 6, written: windows.Filetime{LowDateTime: 7, HighDateTime: 8}}
	for _, test := range []struct {
		stage string
		edit  func(*r17Metadata)
	}{
		{"", func(*r17Metadata) {}},
		{"leaf_metadata_subkeys_changed", func(m *r17Metadata) { m.subkeys++ }},
		{"leaf_metadata_max_subkey_changed", func(m *r17Metadata) { m.maxSubkey++ }},
		{"leaf_metadata_values_changed", func(m *r17Metadata) { m.values++ }},
		{"leaf_metadata_max_value_name_changed", func(m *r17Metadata) { m.maxValueName++ }},
		{"leaf_metadata_max_value_bytes_changed", func(m *r17Metadata) { m.maxValue++ }},
		{"leaf_metadata_security_bytes_changed", func(m *r17Metadata) { m.security++ }},
		{"leaf_metadata_last_write_changed", func(m *r17Metadata) { m.written.LowDateTime++ }},
		{"leaf_metadata_last_write_changed", func(m *r17Metadata) { m.written.HighDateTime++ }},
		{"leaf_metadata_multiple_changed", func(m *r17Metadata) { m.security++; m.values++ }},
		{"leaf_metadata_multiple_changed", func(m *r17Metadata) { m.subkeys++; m.written.HighDateTime++ }},
	} {
		observed := metadata
		test.edit(&observed)
		if r17LeafMetadataMismatchStage(observed, metadata) != test.stage || (observed == metadata) != (test.stage == "") {
			t.Fatal("finite metadata field diagnostic lost full equality or its closed enum")
		}
	}
	for _, qualification := range []string{"", "1"} {
		for _, actions := range []string{"", "true"} {
			for _, runner := range []string{"", "github-hosted"} {
				want := qualification == "1" && actions == "true" && runner == "github-hosted"
				if r17HostedAssessment(qualification, actions, runner) != want {
					t.Fatal("finite assessment must require all existing hosted qualification markers")
				}
			}
		}
	}
	var valid strings.Builder
	for _, fixture := range r17Fixtures {
		for _, right := range r17Rights {
			fmt.Fprintf(&valid, "R17_REGISTRY_ROW fixture_enum=%s right_enum=%s win32_status=5 sentinel_match=false\n", fixture.name, right.name)
		}
	}
	valid.WriteString(r17PassedMarker + "\n")
	body := valid.String()
	if rows, err := r17ParseChild(body); err != nil || len(rows) != 30 {
		t.Fatal("finite registry parser rejected complete synthetic denied observations")
	}
	firstEnd := strings.IndexByte(body, '\n') + 1
	for _, invalid := range []string{
		body[firstEnd:], body[:firstEnd] + body,
		strings.Replace(body, "owner_only", "unknown", 1),
		strings.Replace(body, "win32_status=5", "win32_status=4294967296", 1),
		strings.Replace(body, "win32_status=5", "win32_status=05", 1),
		strings.Replace(body, "sentinel_match=false", "sentinel_match=true", 1),
		strings.Replace(body, "win32_status=5 sentinel_match=false", "win32_status=0 sentinel_match=false", 1),
		strings.Replace(body, "fixture_enum=", "extra=", 1),
		strings.Replace(body, r17PassedMarker, "NATIVE_WINDOWS_CONFINEMENT_PASSED {}", 1),
		body + "R17_REGISTRY_DIAGNOSTIC stage=internal win32_status=13\n",
		body[:len(body)-20], body + strings.Repeat("x", 65536),
	} {
		if _, err := r17ParseChild(invalid); err == nil {
			t.Fatal("finite registry parser accepted incomplete or malformed synthetic observations")
		}
	}
}

func r17Publish(first, second []r17Row) {
	fmt.Fprintf(os.Stdout, "R17_REGISTRY_DEFINITION version=%s sha256=%x\n", r17DefinitionVersion, sha256.Sum256([]byte(r17Definition)))
	for i, rows := range [][]r17Row{first, second} {
		phase := [2]string{"first", "second"}[i]
		for _, row := range rows {
			fmt.Fprintf(os.Stdout, "R17_REGISTRY_OBSERVATION phase=%s fixture_enum=%s right_enum=%s win32_status=%d sentinel_match=%t\n", phase, row.fixture, row.right, row.status, row.match)
		}
	}
}
