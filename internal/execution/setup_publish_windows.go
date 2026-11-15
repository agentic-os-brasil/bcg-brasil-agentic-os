//go:build windows

package execution

import "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"

func publishSetupItem(from, to string) error { return winprivate.Rename(from, to, false) }
