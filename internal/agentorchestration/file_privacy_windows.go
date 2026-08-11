//go:build windows

package agentorchestration

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// validateDurableStateFilePrivacy proves the Windows equivalent of the
// owner-only durable-state boundary. Portable FileMode permission bits are not
// an authority on Windows; the security descriptor is.
func validateDurableStateFilePrivacy(path string) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("inspect durable orchestration state ACL: %w", err)
	}
	if sd == nil || !sd.IsValid() {
		return errors.New("durable orchestration state has no valid security descriptor")
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !owner.IsValid() {
		if err != nil {
			return fmt.Errorf("inspect durable orchestration state owner: %w", err)
		}
		return errors.New("durable orchestration state has no valid owner")
	}
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || tokenUser == nil || tokenUser.User.Sid == nil {
		if err != nil {
			return fmt.Errorf("inspect current Windows user: %w", err)
		}
		return errors.New("current Windows user has no valid SID")
	}
	if !owner.Equals(tokenUser.User.Sid) {
		return errors.New("durable orchestration state is not owned by the current Windows user")
	}

	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		if err != nil {
			return fmt.Errorf("inspect durable orchestration state DACL: %w", err)
		}
		return errors.New("durable orchestration state has no DACL")
	}
	for index := uint16(0); index < dacl.AceCount; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(index), &ace); err != nil {
			return fmt.Errorf("inspect durable orchestration state ACE: %w", err)
		}
		if ace == nil {
			return errors.New("durable orchestration state contains an empty ACE")
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE || ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return fmt.Errorf("durable orchestration state contains unsupported ACE type %d", ace.Header.AceType)
		}
		if ace.Mask == 0 {
			continue
		}
		principal := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !principal.Equals(tokenUser.User.Sid) && !principal.IsWellKnown(windows.WinLocalSystemSid) && !principal.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
			return fmt.Errorf("durable orchestration state grants access to unapproved Windows principal %s", principal.String())
		}
	}
	return nil
}
