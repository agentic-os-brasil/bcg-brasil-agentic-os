//go:build windows

package execution

import "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"

func durableRename(source, target string) error  { return winprivate.Rename(source, target, false) }
func durableReplace(source, target string) error { return winprivate.Rename(source, target, true) }
func durablePublishNoClobber(source, target string) error {
	return winprivate.Rename(source, target, false)
}
