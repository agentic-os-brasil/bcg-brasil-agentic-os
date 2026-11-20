//go:build windows

package packaging

import (
	"context"
	"runtime"
	"unsafe"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"golang.org/x/sys/windows"
)

func observeWeeklyNativeDeclaration(ctx context.Context, path string, target Target) (out processidentity.WeeklyNativeDeclaration, err error) {
	if target.OS != "windows" || target.Arch != runtime.GOARCH || weeklyIdentityContext(ctx) != nil {
		return out, ErrWeeklyObservationUnavailable
	}
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return out, ErrWeeklyObservationUnavailable
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || scheduler.ValidateSingleLink(f) != nil || weeklyCheckEmbeddedPE(f, info.Size(), target.Arch) != nil {
		return out, ErrWeeklyObservationUnavailable
	}
	canonical, err := weeklyIdentityCanonicalPath(f)
	if err != nil || !weeklyIdentityPathsMatch(path, canonical) {
		return out, ErrWeeklyObservationUnavailable
	}
	selected, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return out, ErrWeeklyObservationUnavailable
	}
	file := windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: selected, File: windows.Handle(f.Fd())}
	signature := windows.WinTrustSignatureSettings{Size: uint32(unsafe.Sizeof(windows.WinTrustSignatureSettings{})), Flags: 1, Index: 0, VerifiedSigIndex: ^uint32(0)}
	data := windows.WinTrustData{Size: uint32(unsafe.Sizeof(windows.WinTrustData{})), UIChoice: windows.WTD_UI_NONE, RevocationChecks: windows.WTD_REVOKE_WHOLECHAIN, UnionChoice: windows.WTD_CHOICE_FILE, FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&file), StateAction: windows.WTD_STATEACTION_VERIFY, ProvFlags: windows.WTD_CACHE_ONLY_URL_RETRIEVAL | windows.WTD_REVOCATION_CHECK_CHAIN, SignatureSettings: &signature}
	api := weeklyWindowsTrust{}
	defer func() {
		data.StateAction = windows.WTD_STATEACTION_CLOSE
		if api.verify(&data) != nil || weeklyIdentityContext(ctx) != nil {
			out = processidentity.WeeklyNativeDeclaration{}
			err = ErrWeeklyObservationUnavailable
		}
		runtime.KeepAlive(selected)
		runtime.KeepAlive(file)
		runtime.KeepAlive(signature)
		runtime.KeepAlive(f)
	}()
	if api.verify(&data) != nil || weeklyIdentityContext(ctx) != nil || data.StateData == 0 || data.StateData == windows.InvalidHandle || signature.VerifiedSigIndex != 0 {
		return out, ErrWeeklyObservationUnavailable
	}
	signer, err := api.signer(data.StateData)
	if err != nil {
		return out, ErrWeeklyObservationUnavailable
	}
	out = processidentity.WeeklyNativeDeclaration{Topology: processidentity.WeeklyNativeWindowsStandalone, Architecture: target.Arch, SignatureMode: "embedded_primary_index0", SignerCertificateSHA256: signer, RevocationPolicy: "cache_only_chain"}
	if out.Validate() != nil {
		return processidentity.WeeklyNativeDeclaration{}, ErrWeeklyObservationUnavailable
	}
	return out, nil
}
