package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
)

var updatePlanIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type releaseCapabilityResult struct {
	SchemaVersion        int    `json:"schema_version"`
	Capability           string `json:"capability"`
	State                string `json:"state"`
	Reason               string `json:"reason"`
	ConfirmationRequired bool   `json:"confirmation_required"`
	PlanID               string `json:"plan_id,omitempty"`
	NextAction           string `json:"next_action,omitempty"`
}

func defaultReleaseAuthService() releaseprovider.AuthService {
	return releaseprovider.AuthService{
		Store: releaseprovider.UnavailableStore{},
	}
}

func runAuth(args []string, out, errOut io.Writer, service releaseprovider.AuthService) int {
	if len(args) != 1 {
		fmt.Fprintln(errOut, "usage: bcgos auth <login|status|logout>")
		return ExitUsage
	}
	switch args[0] {
	case "status":
		status, err := service.Status()
		if errors.Is(err, releaseprovider.ErrSecureStoreUnavailable) {
			return writeReleaseUnavailable(out, errOut, "private_release_auth", false, "")
		}
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, struct {
			SchemaVersion int    `json:"schema_version"`
			Capability    string `json:"capability"`
			releaseprovider.AuthStatus
		}{SchemaVersion: 1, Capability: "private_release_auth", AuthStatus: status}, errOut)
	case "login":
		status, err := service.Login(context.Background(), func(authorization releaseprovider.DeviceAuthorization) error {
			fmt.Fprintf(errOut, "Open %s and enter code %s\n", authorization.VerificationURI, authorization.UserCode)
			return nil
		})
		if errors.Is(err, releaseprovider.ErrSecureStoreUnavailable) {
			return writeReleaseUnavailable(out, errOut, "private_release_auth", false, "")
		}
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, struct {
			SchemaVersion int    `json:"schema_version"`
			Capability    string `json:"capability"`
			releaseprovider.AuthStatus
		}{SchemaVersion: 1, Capability: "private_release_auth", AuthStatus: status}, errOut)
	case "logout":
		if err := service.Logout(); errors.Is(err, releaseprovider.ErrSecureStoreUnavailable) {
			return writeReleaseUnavailable(out, errOut, "private_release_auth", false, "")
		} else if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, releaseCapabilityResult{
			SchemaVersion: 1, Capability: "private_release_auth", State: "logged_out",
			Reason: "stored private-release credential removed",
		}, errOut)
	default:
		fmt.Fprintln(errOut, "usage: bcgos auth <login|status|logout>")
		return ExitUsage
	}
}

func runUpdate(args []string, out, errOut io.Writer) int {
	switch {
	case len(args) == 1 && args[0] == "--check":
		return writeReleaseUnavailable(out, errOut, "private_release_update", true, "")
	case len(args) == 2 && args[0] == "--confirm" && updatePlanIDPattern.MatchString(args[1]):
		return writeReleaseUnavailable(out, errOut, "private_release_update", true, args[1])
	default:
		fmt.Fprintln(errOut, "usage: bcgos update <--check|--confirm PLAN_ID>")
		return ExitUsage
	}
}

func writeReleaseUnavailable(out, errOut io.Writer, capability string, confirmation bool, planID string) int {
	result := releaseCapabilityResult{
		SchemaVersion: 1,
		Capability:    capability,
		State:         "unavailable",
		Reason:        "approved operating-system credential store is not configured",
		PlanID:        planID,
	}
	if confirmation {
		result.ConfirmationRequired = true
		result.NextAction = "configure an approved secure store before checking or applying updates"
	}
	if code := writeJSON(out, result, errOut); code != ExitOK {
		return code
	}
	return ExitUnavailable
}
