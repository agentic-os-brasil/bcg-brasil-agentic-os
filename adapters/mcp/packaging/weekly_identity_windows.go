//go:build windows

package packaging

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"unsafe"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
)

var errWeeklyWindowsIdentity = errors.New("weekly Windows identity unavailable")

func verifyWeeklyIdentitySignature(ctx context.Context, identity *WeeklyRuntimeIdentity) error {
	if weeklyIdentityContext(ctx) != nil || identity == nil || identity.file == nil || identity.info == nil || identity.declaration.Validate() != nil || identity.declaration.Topology != processidentity.WeeklyNativeWindowsStandalone || identity.declaration.Architecture != runtime.GOARCH {
		return errWeeklyWindowsIdentity
	}
	if err := weeklyEmbeddedPrimaryPE(identity); err != nil {
		return err
	}
	path, err := windows.UTF16PtrFromString(identity.path)
	if err != nil {
		return errWeeklyWindowsIdentity
	}
	file := windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: path, File: windows.Handle(identity.file.Fd())}
	// WSS_VERIFY_SPECIFIC pins the primary embedded signature; no alternate search.
	signature := windows.WinTrustSignatureSettings{Size: uint32(unsafe.Sizeof(windows.WinTrustSignatureSettings{})), Flags: 1, Index: 0, VerifiedSigIndex: ^uint32(0)}
	data := windows.WinTrustData{Size: uint32(unsafe.Sizeof(windows.WinTrustData{})), UIChoice: windows.WTD_UI_NONE, RevocationChecks: windows.WTD_REVOKE_WHOLECHAIN, UnionChoice: windows.WTD_CHOICE_FILE, FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&file), StateAction: windows.WTD_STATEACTION_VERIFY, ProvFlags: windows.WTD_CACHE_ONLY_URL_RETRIEVAL | windows.WTD_REVOCATION_CHECK_CHAIN, SignatureSettings: &signature}
	err = weeklyVerifyTrust(ctx, weeklyWindowsTrust{}, &data, identity.declaration.SignerCertificateSHA256)
	runtime.KeepAlive(path)
	runtime.KeepAlive(file)
	runtime.KeepAlive(signature)
	runtime.KeepAlive(identity.file)
	return err
}

func weeklyIdentityCanonicalPath(file *os.File) (string, error) {
	return winprivate.FinalLocalPath(file)
}
func weeklyIdentityPathsMatch(a, b string) bool { return strings.EqualFold(a, b) }

func weeklyEmbeddedPrimaryPE(identity *WeeklyRuntimeIdentity) error {
	return weeklyCheckEmbeddedPE(identity.file, identity.info.Size(), identity.declaration.Architecture)
}
func weeklyCheckEmbeddedPE(file io.ReaderAt, size int64, architecture string) error {
	if size < 64 || size > 256<<20 || (architecture != "amd64" && architecture != "arm64") {
		return errWeeklyWindowsIdentity
	}
	var dos [64]byte
	if _, err := file.ReadAt(dos[:], 0); err != nil || string(dos[:2]) != "MZ" {
		return errWeeklyWindowsIdentity
	}
	offset := int64(binary.LittleEndian.Uint32(dos[60:64]))
	var header [264]byte // PE signature, COFF header and the fixed PE32+ optional header.
	if offset < 64 || offset > size-int64(len(header)) {
		return errWeeklyWindowsIdentity
	}
	if _, err := file.ReadAt(header[:], offset); err != nil || string(header[:4]) != "PE\x00\x00" {
		return errWeeklyWindowsIdentity
	}
	machine := uint16(0x8664)
	if architecture == "arm64" {
		machine = 0xaa64
	}
	sections := binary.LittleEndian.Uint16(header[6:8])
	flags := binary.LittleEndian.Uint16(header[22:24])
	optional := header[24:]
	if binary.LittleEndian.Uint16(header[4:6]) != machine || sections == 0 || sections > 96 || binary.LittleEndian.Uint16(header[20:22]) != 240 || flags&0x0002 == 0 || flags&0x2000 != 0 || binary.LittleEndian.Uint16(optional[:2]) != 0x020b || binary.LittleEndian.Uint32(optional[108:112]) != 16 {
		return errWeeklyWindowsIdentity
	}
	if binary.LittleEndian.Uint64(optional[224:232]) != 0 {
		return errWeeklyWindowsIdentity
	} // An interpreter/CLR closure is not this native vector.
	certificateOffset := int64(binary.LittleEndian.Uint32(optional[144:148]))
	certificateSize := int64(binary.LittleEndian.Uint32(optional[148:152]))
	if certificateOffset < offset+int64(len(header))+int64(sections)*40 || certificateOffset%8 != 0 || certificateSize < 8 || certificateOffset > size-certificateSize {
		return errWeeklyWindowsIdentity
	}
	var certificateHeader [8]byte
	if _, err := file.ReadAt(certificateHeader[:], certificateOffset); err != nil {
		return errWeeklyWindowsIdentity
	}
	length := int64(binary.LittleEndian.Uint32(certificateHeader[:4]))
	if length < 8 || length > certificateSize || binary.LittleEndian.Uint16(certificateHeader[4:6]) != 0x0200 || binary.LittleEndian.Uint16(certificateHeader[6:8]) != 0x0002 {
		return errWeeklyWindowsIdentity
	}
	return nil
}

type weeklyTrustAPI interface {
	verify(*windows.WinTrustData) error
	signer(windows.Handle) (string, error)
}
type weeklyWindowsTrust struct{}

func (weeklyWindowsTrust) verify(data *windows.WinTrustData) error {
	return windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
}

// WinVerifyTrustEx has no cancellation parameter. Keep this call synchronously
// owned, close its state on every path, and deny any completion past the fence.
func weeklyVerifyTrust(ctx context.Context, api weeklyTrustAPI, data *windows.WinTrustData, expected string) (err error) {
	if weeklyIdentityContext(ctx) != nil || data == nil || data.SignatureSettings == nil || data.StateAction != windows.WTD_STATEACTION_VERIFY {
		return errWeeklyWindowsIdentity
	}
	defer func() {
		data.StateAction = windows.WTD_STATEACTION_CLOSE
		closeErr := api.verify(data)
		if closeErr != nil || weeklyIdentityContext(ctx) != nil {
			err = errWeeklyWindowsIdentity
		}
	}()
	if api.verify(data) != nil || weeklyIdentityContext(ctx) != nil || data.StateData == 0 || data.StateData == windows.InvalidHandle || data.SignatureSettings.VerifiedSigIndex != 0 {
		return errWeeklyWindowsIdentity
	}
	actual, readErr := api.signer(data.StateData)
	if readErr != nil || actual != expected || weeklyIdentityContext(ctx) != nil {
		return errWeeklyWindowsIdentity
	}
	return nil
}

var (
	weeklyWintrust         = windows.NewLazySystemDLL("wintrust.dll")
	weeklyTrustProvider    = weeklyWintrust.NewProc("WTHelperProvDataFromStateData")
	weeklyTrustSigner      = weeklyWintrust.NewProc("WTHelperGetProvSignerFromChain")
	weeklyTrustCertificate = weeklyWintrust.NewProc("WTHelperGetProvCertFromChain")
)

// Layouts mirror the complete public wintrust.h records on Windows amd64/arm64.
// Provider-owned pointers stay borrowed only until the matching state CLOSE.
type weeklyProviderSigner struct {
	Size               uint32
	VerifyAsOf         windows.Filetime
	ChainCount         uint32
	Chain              *weeklyProviderCertificate
	SignerType         uint32
	Signer             unsafe.Pointer
	Error              uint32
	CounterSignerCount uint32
	CounterSigners     *weeklyProviderSigner
	ChainContext       *windows.CertChainContext
}
type weeklyProviderCertificate struct {
	Size                                                 uint32
	Certificate                                          *windows.CertContext
	Commercial, TrustedRoot, SelfSigned, TestCertificate int32
	RevokedReason, Confidence, Error                     uint32
	TrustListContext                                     unsafe.Pointer
	TrustListSignerCertificate                           int32
	CTLContext                                           unsafe.Pointer
	CTLError                                             uint32
	Cyclic                                               int32
	ChainElement                                         *windows.CertChainElement
}

func (weeklyWindowsTrust) signer(state windows.Handle) (string, error) {
	if (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") || unsafe.Sizeof(weeklyProviderSigner{}) != 64 || unsafe.Sizeof(weeklyProviderCertificate{}) != 88 || unsafe.Sizeof(windows.CertContext{}) != 40 || weeklyTrustProvider.Find() != nil || weeklyTrustSigner.Find() != nil || weeklyTrustCertificate.Find() != nil {
		return "", errWeeklyWindowsIdentity
	}
	provider, _, _ := weeklyTrustProvider.Call(uintptr(state))
	if provider == 0 {
		return "", errWeeklyWindowsIdentity
	}
	signerPointer, _, _ := weeklyTrustSigner.Call(provider, 0, 0, 0)
	if signerPointer == 0 {
		return "", errWeeklyWindowsIdentity
	}
	signer := (*weeklyProviderSigner)(unsafe.Pointer(signerPointer))
	if err := weeklyValidateProviderSigner(signer); err != nil {
		return "", err
	}
	certificatePointer, _, _ := weeklyTrustCertificate.Call(signerPointer, 0)
	if certificatePointer == 0 {
		return "", errWeeklyWindowsIdentity
	}
	selected := (*weeklyProviderCertificate)(unsafe.Pointer(certificatePointer))
	if selected != signer.Chain {
		return "", errWeeklyWindowsIdentity
	}
	return weeklyProviderSignerSHA256(signer, selected)
}
func weeklyValidateProviderSigner(signer *weeklyProviderSigner) error {
	if signer == nil || signer.Size != uint32(unsafe.Sizeof(*signer)) || signer.Error != 0 || signer.SignerType != 0 || signer.Signer == nil || signer.ChainCount == 0 || signer.ChainCount > 16 || signer.Chain == nil || signer.CounterSignerCount > 16 || signer.CounterSignerCount != 0 && signer.CounterSigners == nil {
		return errWeeklyWindowsIdentity
	}
	for _, certificate := range unsafe.Slice(signer.Chain, int(signer.ChainCount)) {
		if certificate.Size != uint32(unsafe.Sizeof(certificate)) || certificate.Certificate == nil || certificate.Error != 0 || certificate.CTLError != 0 || certificate.Cyclic != 0 || certificate.TestCertificate != 0 || certificate.RevokedReason != 0 {
			return errWeeklyWindowsIdentity
		}
	}
	return nil
}
func weeklyProviderSignerSHA256(signer *weeklyProviderSigner, selected *weeklyProviderCertificate) (string, error) {
	if weeklyValidateProviderSigner(signer) != nil || selected == nil || selected != signer.Chain {
		return "", errWeeklyWindowsIdentity
	}
	cert := selected.Certificate
	if cert == nil || cert.EncodingType&windows.X509_ASN_ENCODING == 0 || cert.EncodedCert == nil || cert.Length == 0 || cert.Length > 65536 {
		return "", errWeeklyWindowsIdentity
	}
	der := unsafe.Slice(cert.EncodedCert, int(cert.Length))
	if _, err := x509.ParseCertificate(der); err != nil {
		return "", errWeeklyWindowsIdentity
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:]), nil
}

func (identity *WeeklyRuntimeIdentity) ObserveWindowsChild(ctx context.Context, process, thread, job windows.Handle, pid, tid uint32) (processidentity.WeeklyNativeChild, error) {
	if identity == nil {
		return processidentity.WeeklyNativeChild{}, errWeeklyWindowsIdentity
	}
	identity.mu.Lock()
	defer identity.mu.Unlock()
	if err := identity.verifyCurrentLocked(ctx); err != nil {
		return processidentity.WeeklyNativeChild{}, err
	}
	before, err := processidentity.ObserveWeeklyWindowsChild(ctx, process, thread, job, pid, tid)
	if err != nil {
		return processidentity.WeeklyNativeChild{}, err
	}
	if err := identity.verifyImagePathLocked(ctx, before.ImagePath()); err != nil {
		return processidentity.WeeklyNativeChild{}, err
	}
	after, err := processidentity.ObserveWeeklyWindowsChild(ctx, process, thread, job, pid, tid)
	if err != nil || !before.Same(after) {
		return processidentity.WeeklyNativeChild{}, errWeeklyWindowsIdentity
	}
	if err := identity.verifyCurrentLocked(ctx); err != nil {
		return processidentity.WeeklyNativeChild{}, err
	}
	final, err := processidentity.ObserveWeeklyWindowsChild(ctx, process, thread, job, pid, tid)
	if err != nil || !after.Same(final) {
		return processidentity.WeeklyNativeChild{}, errWeeklyWindowsIdentity
	}
	return final, nil
}
func (identity *WeeklyRuntimeIdentity) VerifyWindowsTerminal(ctx context.Context, process, thread, job windows.Handle, pid, tid uint32, expected processidentity.WeeklyNativeChild) error {
	if identity == nil {
		return errWeeklyWindowsIdentity
	}
	identity.mu.Lock()
	defer identity.mu.Unlock()
	if err := identity.verifyCurrentLocked(ctx); err != nil {
		return err
	}
	if err := processidentity.VerifyWeeklyWindowsTerminal(ctx, process, thread, job, pid, tid, expected); err != nil {
		return err
	}
	if err := identity.verifyImagePathLocked(ctx, expected.ImagePath()); err != nil {
		return err
	}
	if err := identity.verifyCurrentLocked(ctx); err != nil {
		return err
	}
	return processidentity.VerifyWeeklyWindowsTerminal(ctx, process, thread, job, pid, tid, expected)
}
