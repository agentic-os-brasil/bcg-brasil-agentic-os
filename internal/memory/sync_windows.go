//go:build windows

package memory

import "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"

func syncDirectory(path string) error { return winprivate.Sync(path) }
