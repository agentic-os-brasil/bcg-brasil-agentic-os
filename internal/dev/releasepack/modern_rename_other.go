//go:build !windows

package releasepack

import (
	"context"
	"os"
)

func renameModernCandidate(_ context.Context, stage, output string) error {
	return os.Rename(stage, output)
}
