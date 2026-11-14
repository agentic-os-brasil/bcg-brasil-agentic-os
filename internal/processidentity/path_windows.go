//go:build windows

package processidentity

import (
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
)

func executableFilePath(file *os.File) (string, error) {
	return winprivate.FinalLocalPath(file)
}
