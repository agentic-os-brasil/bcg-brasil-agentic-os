package docling

import (
	"errors"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"os"
	"path/filepath"
)

func bridgeEnvironment(work string) []string {
	return []string{"PATH=/usr/bin:/bin", "HOME=" + filepath.Join(work, "home"), "TMPDIR=" + filepath.Join(work, "tmp"), "XDG_CACHE_HOME=" + filepath.Join(work, "cache"), "XDG_CONFIG_HOME=" + filepath.Join(work, "config"), "XDG_DATA_HOME=" + filepath.Join(work, "data"), "HF_HOME=" + filepath.Join(work, "models"), "HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1", "DOCLING_ARTIFACTS_PATH=" + filepath.Join(work, "models"), "OMP_NUM_THREADS=1", "OPENBLAS_NUM_THREADS=1", "VECLIB_MAXIMUM_THREADS=1", "LANG=en_US.UTF-8"}
}

var errExecutionSnapshot = errors.New("execution_snapshot_unavailable")

func prepareExecution(dataRoot string, pack verifiedPack) (string, string, error) {
	executions := filepath.Join(dataRoot, "ingestion", "docling", "executions")
	if err := scheduler.EnsurePrivateDirectory(executions); err != nil {
		return "", "", err
	}
	work, err := os.MkdirTemp(executions, "conversion-")
	if err != nil {
		return "", "", err
	}
	snapshot := filepath.Join(work, "pack")
	if err = materialize(pack.entries, snapshot); err != nil {
		os.RemoveAll(work)
		return "", "", fmt.Errorf("%w: %w", errExecutionSnapshot, err)
	}
	for _, directory := range []string{"home", "cache", "models", "tmp", "config", "data"} {
		if err = os.Mkdir(filepath.Join(work, directory), 0700); err != nil {
			os.RemoveAll(work)
			return "", "", err
		}
	}
	return work, snapshot, nil
}
