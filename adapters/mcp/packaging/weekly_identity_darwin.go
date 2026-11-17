//go:build darwin

package packaging

import (
	"context"
	"errors"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func weeklyIdentityCanonicalPath(file *os.File) (string, error) {
	if file == nil {
		return "", errWeeklyIdentityUnavailable
	}
	path := file.Name()
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", errWeeklyIdentityUnavailable
	}
	return path, nil
}
func weeklyIdentityPathsMatch(a, b string) bool { return a == b }
func weeklyDarwinRequirement(d processidentity.WeeklyNativeDeclaration) (string, error) {
	if d.Validate() != nil || d.Topology != processidentity.WeeklyNativeDarwinStandalone {
		return "", errWeeklyIdentityUnavailable
	}
	return fmt.Sprintf(`anchor apple generic and certificate leaf[subject.OU] = "%s" and identifier "%s" and cdhash H"%s"`, d.TeamID, d.SigningIdentifier, d.CodeDirectoryHash), nil
}
func weeklyDarwinStaticArguments(d processidentity.WeeklyNativeDeclaration, path string) ([]string, error) {
	requirement, err := weeklyDarwinRequirement(d)
	if err != nil || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n") {
		return nil, errWeeklyIdentityUnavailable
	}
	return []string{"--verify", "--verbose=1", "--test-requirement", "=" + requirement, "--", path}, nil
}
func weeklyDarwinDynamicArguments(d processidentity.WeeklyNativeDeclaration, pid int) ([]string, error) {
	requirement, err := weeklyDarwinRequirement(d)
	if err != nil || pid <= 0 || pid > 2147483647 {
		return nil, errWeeklyIdentityUnavailable
	}
	return []string{"--verify", "--verbose=1", "--test-requirement", "=" + requirement, "+" + strconv.Itoa(pid)}, nil
}
func runWeeklyIdentityCodesign(ctx context.Context, args []string) error {
	if err := weeklyIdentityContext(ctx); err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// No inherited profile/environment, shell, input or certificate extraction.
	// The reviewed transport owns cancellation, pipe joins, group closure and Wait.
	_, err := runWeeklyModelProcess(bounded, weeklyModelProcess{Path: "/usr/bin/codesign", Args: args, Directory: "/", Environment: []string{"LANG=C", "LC_ALL=C", "PATH=/usr/bin:/bin"}, MaxOutput: 4096, MaxStderr: 4096})
	if err != nil || bounded.Err() != nil {
		return errors.Join(errWeeklyIdentityUnavailable, err, bounded.Err())
	}
	return weeklyIdentityContext(ctx)
}
func verifyWeeklyIdentitySignature(ctx context.Context, i *WeeklyRuntimeIdentity) error {
	if i == nil || i.declaration.Topology != processidentity.WeeklyNativeDarwinStandalone {
		return errWeeklyIdentityUnavailable
	}
	args, err := weeklyDarwinStaticArguments(i.declaration, i.path)
	if err != nil {
		return err
	}
	return runWeeklyIdentityCodesign(ctx, args)
}
func (i *WeeklyRuntimeIdentity) ObserveChild(ctx context.Context, child *os.Process, parent int, startedAfter time.Time) (processidentity.WeeklyNativeChild, error) {
	if i == nil {
		return processidentity.WeeklyNativeChild{}, errWeeklyIdentityUnavailable
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if err := i.verifyCurrentLocked(ctx); err != nil {
		return processidentity.WeeklyNativeChild{}, err
	}
	before, err := processidentity.ObserveWeeklyNativeChild(child, parent, startedAfter)
	if err != nil {
		return processidentity.WeeklyNativeChild{}, err
	}
	args, err := weeklyDarwinDynamicArguments(i.declaration, before.PID())
	if err != nil {
		return processidentity.WeeklyNativeChild{}, err
	}
	if err := runWeeklyIdentityCodesign(ctx, args); err != nil {
		return processidentity.WeeklyNativeChild{}, err
	}
	after, err := processidentity.ObserveWeeklyNativeChild(child, parent, startedAfter)
	if err != nil || !before.Same(after) {
		return processidentity.WeeklyNativeChild{}, errWeeklyIdentityUnavailable
	}
	if err := i.verifyCurrentLocked(ctx); err != nil {
		return processidentity.WeeklyNativeChild{}, err
	}
	return after, nil
}
func (i *WeeklyRuntimeIdentity) VerifyTerminal(ctx context.Context, child *os.Process, expected processidentity.WeeklyNativeChild) error {
	if i == nil {
		return errWeeklyIdentityUnavailable
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if err := i.verifyCurrentLocked(ctx); err != nil {
		return err
	}
	if err := processidentity.VerifyWeeklyNativeTerminal(child, expected); err != nil {
		return err
	}
	if err := i.verifyCurrentLocked(ctx); err != nil {
		return err
	}
	return processidentity.VerifyWeeklyNativeTerminal(child, expected)
}
