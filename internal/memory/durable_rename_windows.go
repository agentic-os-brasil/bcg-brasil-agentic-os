//go:build windows

package memory

import "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"

func durableRename(source, target string) error { return winprivate.Rename(source, target, false) }
