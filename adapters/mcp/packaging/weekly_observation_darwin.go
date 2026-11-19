//go:build darwin

package packaging

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
)

func observeWeeklyNativeDeclaration(ctx context.Context, path string, target Target) (processidentity.WeeklyNativeDeclaration, error) {
	var out processidentity.WeeklyNativeDeclaration
	if target.OS != "darwin" || weeklyIdentityContext(ctx) != nil {
		return out, ErrWeeklyObservationUnavailable
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// codesign displays its public metadata on stderr. Reuse the original
	// bounded buffers and owned process cancellation; never invoke a shell.
	cmd := exec.CommandContext(bounded, "/usr/bin/codesign", "--display", "--verbose=4", "--", path)
	cmd.Dir = "/"
	cmd.Env = []string{"LANG=C", "LC_ALL=C", "PATH=/usr/bin:/bin"}
	cmd.WaitDelay = 2 * time.Second
	configureNativeProcess(cmd)
	stdout := weeklyModelOutput{limit: 4096}
	stderr := weeklyModelOutput{limit: 4096}
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if cmd.Run() != nil || bounded.Err() != nil {
		return out, ErrWeeklyObservationUnavailable
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(stderr.Bytes()), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "Identifier", "TeamIdentifier", "CDHash":
			if _, duplicate := fields[key]; duplicate {
				return out, ErrWeeklyObservationUnavailable
			}
			fields[key] = value
		}
	}
	out = processidentity.WeeklyNativeDeclaration{Topology: processidentity.WeeklyNativeDarwinStandalone, TeamID: fields["TeamIdentifier"], SigningIdentifier: fields["Identifier"], CodeDirectoryHash: fields["CDHash"], Architecture: target.Arch}
	if out.Validate() != nil {
		return processidentity.WeeklyNativeDeclaration{}, ErrWeeklyObservationUnavailable
	}
	return out, nil
}
